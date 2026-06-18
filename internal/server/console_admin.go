package server

import (
	"net/http"
	"net/url"

	"github.com/callmemhz/milo/internal/auth"
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
	}
	s.render(w, "admin", data)
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
