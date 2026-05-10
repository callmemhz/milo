package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/pkg/api"
)

func (s *Server) registerUsersRoutes(r chi.Router) {
	r.Post("/v1/users", s.handleCreateUser)
	r.Get("/v1/users", s.handleListUsers)
	r.Get("/v1/users/{username}", s.handleGetUser)
	r.Delete("/v1/users/{username}", s.handleDeleteUser)
	r.Post("/v1/users/{username}/tokens", s.handleCreateUserToken)
	r.Get("/v1/users/{username}/tokens", s.handleListUserTokens)
	r.Delete("/v1/users/{username}/tokens/{id}", s.handleRevokeUserToken)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if err := auth.RequireAdmin(id); err != nil {
		writeError(w, api.New(api.ErrForbidden, err.Error()))
		return
	}
	var req api.CreateUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad json"))
		return
	}
	if !validUsername(req.Username) {
		writeError(w, api.New(api.ErrInvalid, "invalid username"))
		return
	}
	u, err := s.Store.CreateUser(r.Context(), req.Username, req.IsAdmin)
	if err != nil {
		writeError(w, api.New(api.ErrConflict, "username in use"))
		return
	}
	writeJSON(w, http.StatusCreated, api.UserResp{ID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if err := auth.RequireAdmin(id); err != nil {
		writeError(w, api.New(api.ErrForbidden, err.Error()))
		return
	}
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]api.UserResp, 0, len(users))
	for _, u := range users {
		out = append(out, api.UserResp{ID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrUnauthorized, "no user"))
		return
	}
	name := chi.URLParam(r, "username")
	if !id.User.IsAdmin && id.User.Username != name {
		writeError(w, api.New(api.ErrForbidden, "not self / not admin"))
		return
	}
	u, err := s.Store.GetUserByUsername(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "user not found"))
		return
	}
	writeJSON(w, http.StatusOK, api.UserResp{ID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if err := auth.RequireAdmin(id); err != nil {
		writeError(w, api.New(api.ErrForbidden, err.Error()))
		return
	}
	name := chi.URLParam(r, "username")
	u, err := s.Store.GetUserByUsername(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "user not found"))
		return
	}
	if err := s.Store.SoftDeleteUser(r.Context(), u.ID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateUserToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrUnauthorized, "no user"))
		return
	}
	name := chi.URLParam(r, "username")
	if !id.User.IsAdmin && id.User.Username != name {
		writeError(w, api.New(api.ErrForbidden, "not self / not admin"))
		return
	}
	target, err := s.Store.GetUserByUsername(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "user not found"))
		return
	}
	var req api.CreateTokenReq
	_ = json.NewDecoder(r.Body).Decode(&req) // body optional
	plaintext, err := auth.Generate()
	if err != nil {
		writeError(w, err)
		return
	}
	tk, err := s.Store.CreateUserToken(r.Context(), target.ID, auth.Hash(plaintext), req.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.CreateTokenResp{ID: tk.ID, Token: plaintext, Name: req.Name})
}

func (s *Server) handleListUserTokens(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrUnauthorized, "no user"))
		return
	}
	name := chi.URLParam(r, "username")
	if !id.User.IsAdmin && id.User.Username != name {
		writeError(w, api.New(api.ErrForbidden, "not self / not admin"))
		return
	}
	target, err := s.Store.GetUserByUsername(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "user not found"))
		return
	}
	tokens, err := s.Store.ListUserTokens(r.Context(), target.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]api.TokenResp, 0, len(tokens))
	for _, tk := range tokens {
		nm := ""
		if tk.Name != nil {
			nm = *tk.Name
		}
		lu := ""
		if tk.LastUsedAt != nil {
			lu = tk.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, api.TokenResp{ID: tk.ID, Name: nm, Kind: tk.Kind, LastUsedAt: lu})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRevokeUserToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrUnauthorized, "no user"))
		return
	}
	name := chi.URLParam(r, "username")
	if !id.User.IsAdmin && id.User.Username != name {
		writeError(w, api.New(api.ErrForbidden, "not self / not admin"))
		return
	}
	tokenID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad id"))
		return
	}
	if err := s.Store.RevokeToken(r.Context(), tokenID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validUsername(s string) bool {
	if len(s) < 1 || len(s) > 32 {
		return false
	}
	if s[0] == '-' {
		return false
	}
	for _, c := range s {
		if !(c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}
