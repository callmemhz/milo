package server

import (
	"net/http"

	"github.com/callmemhz/milo/internal/auth"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.Version})
}

func (s *Server) handleWhoami(w http.ResponseWriter, r *http.Request) {
	id, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"code": "internal", "message": "missing identity"})
		return
	}
	out := map[string]any{"token_kind": id.Token.Kind}
	if id.User != nil {
		out["username"] = id.User.Username
		out["is_admin"] = id.User.IsAdmin
	}
	if id.AppID != nil {
		a, err := s.Store.GetAppByID(r.Context(), *id.AppID)
		if err == nil {
			out["scope"] = a.Name
		}
	}
	writeJSON(w, http.StatusOK, out)
}
