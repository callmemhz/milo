package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/docker"
)

// usersFlash redirects back to the users page with an encoded message/error.
func usersFlash(w http.ResponseWriter, r *http.Request, key, msg string) {
	http.Redirect(w, r, "/console/users?"+key+"="+url.QueryEscape(msg), http.StatusFound)
}

// consoleAdmin renders the admin overview: docker host status + global counts.
func (s *Server) consoleAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := auth.IdentityFromContext(ctx)

	apps, _ := s.Store.ListApps(ctx)
	addons, _ := s.Store.ListAddons(ctx)
	users, _ := s.Store.ListUsers(ctx)

	data := map[string]any{
		"User":       id.User.Username,
		"Admin":      true,
		"CSRF":       s.ensureCSRF(w, r),
		"AppCount":   len(apps),
		"AddonCount": len(addons),
		"UserCount":  len(users),
		"Version":    s.Version,
	}
	ncpu := 0
	if s.Runtime != nil {
		if hi, err := s.Runtime.Info(ctx); err == nil {
			ncpu = hi.NCPU
			data["Host"] = map[string]any{
				"ServerVersion":     hi.ServerVersion,
				"OS":                hi.OperatingSystem,
				"Arch":              hi.Architecture,
				"NCPU":              hi.NCPU,
				"MemTotal":          humanBytes(uint64(hi.MemTotal)),
				"Containers":        hi.Containers,
				"ContainersRunning": hi.ContainersRunning,
				"Images":            hi.Images,
			}
		}
		if du, err := s.Runtime.DiskUsage(ctx); err == nil {
			data["Disk"] = map[string]any{
				"Images":     humanBytes(uint64(du.ImagesSize)),
				"Containers": humanBytes(uint64(du.ContainersSize)),
				"Volumes":    humanBytes(uint64(du.VolumesSize)),
				"BuildCache": humanBytes(uint64(du.BuildCacheSize)),
				"Total":      humanBytes(uint64(du.ImagesSize + du.ContainersSize + du.VolumesSize + du.BuildCacheSize)),
			}
		}
	}
	// Host load (read from /proc; reflects the docker host/VM).
	if hl := docker.ReadHostLoad(); hl.OK {
		loadPct := 0.0
		if ncpu > 0 {
			loadPct = hl.Load1 / float64(ncpu) * 100
		}
		ld := map[string]any{
			"LoadPct": loadPct,
			"LoadNum": fmt.Sprintf("%.2f / %.2f / %.2f", hl.Load1, hl.Load5, hl.Load15),
		}
		if hl.MemTotal > 0 {
			used := hl.MemTotal - hl.MemAvail
			ld["MemPct"] = float64(used) / float64(hl.MemTotal) * 100
			ld["MemNum"] = humanBytes(used) + " / " + humanBytes(hl.MemTotal)
		}
		data["Load"] = ld
	}
	// Host disk free for the data filesystem.
	if hd := docker.ReadHostDisk("/var/lib/milo"); hd.OK && hd.Total > 0 {
		used := hd.Total - hd.Free
		data["DiskFree"] = map[string]any{
			"DiskPct": float64(used) / float64(hd.Total) * 100,
			"DiskNum": humanBytes(used) + " / " + humanBytes(hd.Total) + "（剩 " + humanBytes(hd.Free) + "）",
		}
	}
	s.render(w, "admin", data)
}

// consoleImages lists local docker images for admin management.
func (s *Server) consoleImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := auth.IdentityFromContext(ctx)

	type imgRow struct {
		ID     string
		Short  string
		Tags   string
		Size   string
		InUse  bool
		UsedBy string
	}
	var rows []imgRow
	if s.Runtime != nil {
		usage, _ := s.Runtime.ImageUsage(ctx)
		if imgs, err := s.Runtime.ImageList(ctx); err == nil {
			for _, im := range imgs {
				tags := "<none>"
				if len(im.RepoTags) > 0 {
					tags = strings.Join(im.RepoTags, ", ")
				}
				short := strings.TrimPrefix(im.ID, "sha256:")
				if len(short) > 12 {
					short = short[:12]
				}
				users := usage[im.ID]
				rows = append(rows, imgRow{
					ID: im.ID, Short: short, Tags: tags, Size: humanBytes(uint64(im.Size)),
					InUse: len(users) > 0, UsedBy: strings.Join(users, ", "),
				})
			}
		}
	}
	s.render(w, "images", map[string]any{
		"User":   id.User.Username,
		"Admin":  true,
		"CSRF":   s.ensureCSRF(w, r),
		"Images": rows,
		"Flash":  r.URL.Query().Get("msg"),
		"Error":  r.URL.Query().Get("err"),
	})
}

// consoleImageDelete removes one or more selected images (multi-select batch).
func (s *Server) consoleImageDelete(w http.ResponseWriter, r *http.Request) {
	imagesFlash := func(key, msg string) {
		http.Redirect(w, r, "/console/admin/images?"+key+"="+url.QueryEscape(msg), http.StatusFound)
	}
	if !s.checkCSRF(r) {
		imagesFlash("err", "会话过期，请重试")
		return
	}
	if err := r.ParseForm(); err != nil {
		imagesFlash("err", "表单解析失败")
		return
	}
	ids := r.Form["ids"]
	force := r.FormValue("force") == "on"
	if s.Runtime == nil || len(ids) == 0 {
		imagesFlash("err", "未选择任何镜像")
		return
	}
	ok, failed := 0, 0
	for _, id := range ids {
		if id == "" {
			continue
		}
		if err := s.Runtime.ImageRemove(r.Context(), id, force); err != nil {
			failed++
		} else {
			ok++
		}
	}
	msg := fmt.Sprintf("已删除 %d 个镜像", ok)
	if failed > 0 {
		msg += fmt.Sprintf("，%d 个失败（可能被容器占用，可勾选强制）", failed)
	}
	imagesFlash("msg", msg)
}

type userRow struct {
	Username    string
	IsAdmin     bool
	HasPassword bool
	Frozen      bool
	Created     string
}

func (s *Server) consoleUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := auth.IdentityFromContext(ctx)
	users, _ := s.Store.ListUsers(ctx)

	rows := make([]userRow, 0, len(users))
	for _, u := range users {
		hash, _ := s.Store.GetUserPasswordHash(ctx, u.ID)
		frozen, _ := s.Store.IsUserFrozen(ctx, u.ID)
		rows = append(rows, userRow{
			Username:    u.Username,
			IsAdmin:     u.IsAdmin,
			HasPassword: hash != "",
			Frozen:      frozen,
			Created:     u.CreatedAt.Format("2006-01-02"),
		})
	}
	s.render(w, "users", map[string]any{
		"User":  id.User.Username,
		"Admin": true,
		"Self":  id.User.Username,
		"CSRF":  s.ensureCSRF(w, r),
		"Users": rows,
		"Flash": r.URL.Query().Get("msg"),
		"Error": r.URL.Query().Get("err"),
	})
}

func (s *Server) consoleUserCreate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		usersFlash(w, r, "err", "会话过期，请重试")
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	isAdmin := r.FormValue("is_admin") == "on"

	if !validUsername(username) {
		usersFlash(w, r, "err", "用户名不合法")
		return
	}
	if len(password) < 8 {
		usersFlash(w, r, "err", "密码至少 8 位")
		return
	}
	u, err := s.Store.CreateUser(r.Context(), username, isAdmin)
	if err != nil {
		usersFlash(w, r, "err", "用户名已存在")
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		usersFlash(w, r, "err", "设密失败")
		return
	}
	_ = s.Store.SetUserPassword(r.Context(), u.ID, hash)
	usersFlash(w, r, "msg", "已创建 "+username)
}

func (s *Server) consoleUserSetPassword(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		usersFlash(w, r, "err", "会话过期，请重试")
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	if len(password) < 8 {
		usersFlash(w, r, "err", "密码至少 8 位")
		return
	}
	u, err := s.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		usersFlash(w, r, "err", "用户不存在")
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		usersFlash(w, r, "err", "设密失败")
		return
	}
	_ = s.Store.SetUserPassword(r.Context(), u.ID, hash)
	usersFlash(w, r, "msg", "已重置 "+username+" 的密码")
}

func (s *Server) consoleUserFreeze(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		usersFlash(w, r, "err", "会话过期，请重试")
		return
	}
	id, _ := auth.IdentityFromContext(r.Context())
	username := r.FormValue("username")
	want := r.FormValue("frozen") == "true"
	if username == id.User.Username {
		usersFlash(w, r, "err", "不能冻结自己")
		return
	}
	u, err := s.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		usersFlash(w, r, "err", "用户不存在")
		return
	}
	if err := s.Store.SetUserFrozen(r.Context(), u.ID, want); err != nil {
		usersFlash(w, r, "err", "操作失败")
		return
	}
	if want {
		// Kick the user out immediately by dropping all browser sessions.
		_ = s.Store.DeleteUserSessions(r.Context(), u.ID)
		usersFlash(w, r, "msg", "已冻结 "+username+"（账号保留，无法登录/调用）")
		return
	}
	usersFlash(w, r, "msg", "已解冻 "+username)
}

func (s *Server) consoleUserDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		usersFlash(w, r, "err", "会话过期，请重试")
		return
	}
	id, _ := auth.IdentityFromContext(r.Context())
	username := r.FormValue("username")
	if username == id.User.Username {
		usersFlash(w, r, "err", "不能删除自己")
		return
	}
	u, err := s.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		usersFlash(w, r, "err", "用户不存在")
		return
	}
	if u.IsAdmin {
		if n, _ := s.Store.CountAdmins(r.Context()); n <= 1 {
			usersFlash(w, r, "err", "不能删除最后一个管理员")
			return
		}
	}
	_ = s.Store.SoftDeleteUser(r.Context(), u.ID)
	_ = s.Store.DeleteUserSessions(r.Context(), u.ID)
	usersFlash(w, r, "msg", "已删除 "+username)
}
