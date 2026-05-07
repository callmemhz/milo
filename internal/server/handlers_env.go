package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo-apps-kit/internal/auth"
	"github.com/callmemhz/milo-apps-kit/pkg/api"
)

func (s *Server) registerEnvRoutes(r chi.Router) {
	r.Get("/v1/apps/{app}/env", s.handleGetEnv)
	r.Put("/v1/apps/{app}/env", s.handlePutEnv)
	r.Patch("/v1/apps/{app}/env", s.handlePatchEnv)
}

// loadOwnedApp validates the URL {app}, ensures the caller is owner-or-admin,
// and returns the app's id. On failure it writes the error and returns ok=false.
func (s *Server) loadOwnedApp(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, _ := auth.IdentityFromContext(r.Context())
	name := chi.URLParam(r, "app")
	a, err := s.Store.GetAppByName(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "app not found"))
		return 0, false
	}
	if err := auth.RequireOwnerOrAdmin(r.Context(), s.Store, id, a.ID); err != nil {
		writeError(w, api.New(api.ErrForbidden, err.Error()))
		return 0, false
	}
	return a.ID, true
}

func (s *Server) handleGetEnv(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	env, err := s.Store.GetAppEnv(r.Context(), appID)
	if err != nil {
		writeError(w, err)
		return
	}
	if env == nil {
		env = map[string]string{}
	}
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) handlePutEnv(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad json"))
		return
	}
	if req == nil {
		req = map[string]string{}
	}
	if err := s.Store.ReplaceAppEnv(r.Context(), appID, req); err != nil {
		writeError(w, err)
		return
	}
	s.maybeRedeployCurrent(r.Context(), appID)
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handlePatchEnv(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	var req api.EnvPatchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad json"))
		return
	}
	for k, v := range req.Set {
		if err := s.Store.SetAppEnvVar(r.Context(), appID, k, v); err != nil {
			writeError(w, err)
			return
		}
	}
	for _, k := range req.Unset {
		_ = s.Store.DeleteAppEnvVar(r.Context(), appID, k)
	}
	s.maybeRedeployCurrent(r.Context(), appID)
	env, _ := s.Store.GetAppEnv(r.Context(), appID)
	if env == nil {
		env = map[string]string{}
	}
	writeJSON(w, http.StatusOK, env)
}
