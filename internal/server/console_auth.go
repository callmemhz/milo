package server

import (
	"net/http"
	"time"

	"github.com/callmemhz/milo/internal/auth"
)

// ensureCSRF returns the request's CSRF token, minting and setting the cookie if
// absent. Double-submit pattern: the same value goes into the cookie and into a
// hidden form field; checkCSRF compares them on POST. SameSite=Lax on the
// session cookie is the primary defense; this is belt-and-suspenders.
func (s *Server) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && c.Value != "" {
		return c.Value
	}
	tok, _ := auth.Generate()
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookie, Value: tok, Path: "/", HttpOnly: false,
		Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(sessionTTL),
	})
	return tok
}

func (s *Server) checkCSRF(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" {
		return false
	}
	return r.FormValue("_csrf") == c.Value
}

func (s *Server) consoleLoginForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.resolveSession(w, r); ok {
		http.Redirect(w, r, "/console", http.StatusFound)
		return
	}
	s.render(w, "login", map[string]any{"CSRF": s.ensureCSRF(w, r)})
}

func (s *Server) consoleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.render(w, "login", map[string]any{"CSRF": s.ensureCSRF(w, r), "Error": "会话已过期，请重试"})
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	fail := func() {
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, "login", map[string]any{"CSRF": s.ensureCSRF(w, r), "Error": "用户名或密码错误"})
	}

	u, err := s.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		fail()
		return
	}
	hash, err := s.Store.GetUserPasswordHash(r.Context(), u.ID)
	if err != nil || !auth.CheckPassword(hash, password) {
		fail()
		return
	}
	if frozen, _ := s.Store.IsUserFrozen(r.Context(), u.ID); frozen {
		w.WriteHeader(http.StatusForbidden)
		s.render(w, "login", map[string]any{"CSRF": s.ensureCSRF(w, r), "Error": "账号已被冻结，请联系管理员"})
		return
	}

	token, err := auth.Generate()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if err := s.Store.CreateSession(r.Context(), u.ID, auth.Hash(token), time.Now().Add(sessionTTL)); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, token)
	http.Redirect(w, r, "/console", http.StatusFound)
}

func (s *Server) consoleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.Store.DeleteSession(r.Context(), auth.Hash(c.Value))
	}
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/console/login", http.StatusFound)
}
