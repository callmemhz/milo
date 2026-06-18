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
	if s.Runtime != nil {
		if hi, err := s.Runtime.Info(ctx); err == nil {
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
		used := hl.MemTotal - hl.MemAvail
		data["Load"] = map[string]any{
			"L1":       fmt.Sprintf("%.2f", hl.Load1),
			"L5":       fmt.Sprintf("%.2f", hl.Load5),
			"L15":      fmt.Sprintf("%.2f", hl.Load15),
			"MemUsed":  humanBytes(used),
			"MemTotal": humanBytes(hl.MemTotal),
		}
	}
	// Host disk free for the data filesystem.
	if hd := docker.ReadHostDisk("/var/lib/milo"); hd.OK {
		data["DiskFree"] = map[string]any{
			"Free":  humanBytes(hd.Free),
			"Total": humanBytes(hd.Total),
			"Used":  humanBytes(hd.Total - hd.Free),
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

func (s *Server) consoleImageDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Redirect(w, r, "/console/admin/images?err="+url.QueryEscape("会话过期，请重试"), http.StatusFound)
		return
	}
	idArg := r.FormValue("id")
	force := r.FormValue("force") == "on"
	if s.Runtime == nil || idArg == "" {
		http.Redirect(w, r, "/console/admin/images", http.StatusFound)
		return
	}
	if err := s.Runtime.ImageRemove(r.Context(), idArg, force); err != nil {
		http.Redirect(w, r, "/console/admin/images?err="+url.QueryEscape("删除失败（可能被容器占用，可勾选强制）"), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/console/admin/images?msg="+url.QueryEscape("已删除镜像"), http.StatusFound)
}

type userRow struct {
	Username    string
	IsAdmin     bool
	HasPassword bool
	Created     string
}

func (s *Server) consoleUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := auth.IdentityFromContext(ctx)
	users, _ := s.Store.ListUsers(ctx)

	rows := make([]userRow, 0, len(users))
	for _, u := range users {
		hash, _ := s.Store.GetUserPasswordHash(ctx, u.ID)
		rows = append(rows, userRow{
			Username:    u.Username,
			IsAdmin:     u.IsAdmin,
			HasPassword: hash != "",
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
