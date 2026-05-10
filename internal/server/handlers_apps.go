package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/store"
	"github.com/callmemhz/milo/pkg/api"
)

const (
	defaultPort        = 8080
	defaultHealthPath  = "/"
	defaultHealthTOSec = 30
	defaultCPULimit    = 0.5
	defaultMemMB       = 512
)

var reservedAppNames = map[string]bool{
	"api": true, "www": true, "admin": true, "auth": true,
	"docs": true, "milo": true, "caddy": true, "localhost": true,
}

// validAppName matches spec §4.1: 2..32 chars, [a-z0-9-], leading [a-z], trailing [a-z0-9], not in denylist.
func validAppName(s string) bool {
	if len(s) < 2 || len(s) > 32 {
		return false
	}
	if reservedAppNames[s] {
		return false
	}
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	last := s[len(s)-1]
	if !((last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')) {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func (s *Server) registerAppsRoutes(r chi.Router) {
	r.Post("/v1/apps", s.handleCreateApp)
	r.Get("/v1/apps", s.handleListApps)
	r.Get("/v1/apps/{app}", s.handleGetApp)
	r.Patch("/v1/apps/{app}", s.handleUpdateApp)
	// DELETE wired in M10 once orchestrator exists.
}

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrUnauthorized, "no user"))
		return
	}
	var req api.CreateAppReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad json"))
		return
	}
	if !validAppName(req.Name) {
		writeError(w, api.New(api.ErrInvalid, "invalid app name"))
		return
	}
	if req.Port == 0 {
		req.Port = defaultPort
	}
	if req.HealthPath == "" {
		req.HealthPath = defaultHealthPath
	}
	if req.HealthTimeoutSec == 0 {
		req.HealthTimeoutSec = defaultHealthTOSec
	}
	if req.CPULimit == 0 {
		req.CPULimit = defaultCPULimit
	}
	if req.MemoryLimitMB == 0 {
		req.MemoryLimitMB = defaultMemMB
	}

	a, err := s.Store.CreateApp(r.Context(), req.Name, req.Port, req.HealthPath, req.HealthTimeoutSec, req.CPULimit, req.MemoryLimitMB)
	if err != nil {
		writeError(w, api.New(api.ErrConflict, "name in use"))
		return
	}

	owners := []int64{id.User.ID}
	if id.User.IsAdmin {
		for _, name := range req.Owners {
			if name == id.User.Username {
				continue
			}
			u, err := s.Store.GetUserByUsername(r.Context(), name)
			if err != nil {
				writeError(w, api.New(api.ErrInvalid, "owner not found: "+name))
				return
			}
			owners = append(owners, u.ID)
		}
	} else {
		for _, name := range req.Owners {
			if name != id.User.Username {
				writeError(w, api.New(api.ErrForbidden, "non-admin can only own their own apps"))
				return
			}
		}
	}
	for _, uid := range owners {
		_ = s.Store.AddOwner(r.Context(), a.ID, uid)
	}

	writeJSON(w, http.StatusCreated, toAppResp(r.Context(), s.Store, a))
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrUnauthorized, "no user"))
		return
	}
	var apps []store.App
	var err error
	if id.User.IsAdmin {
		apps, err = s.Store.ListApps(r.Context())
	} else {
		apps, err = s.Store.ListAppsByOwner(r.Context(), id.User.ID)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]api.AppResp, 0, len(apps))
	for _, a := range apps {
		out = append(out, toAppResp(r.Context(), s.Store, a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	name := chi.URLParam(r, "app")
	a, err := s.Store.GetAppByName(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "app not found"))
		return
	}
	if err := auth.RequireOwnerOrAdmin(r.Context(), s.Store, id, a.ID); err != nil {
		writeError(w, api.New(api.ErrForbidden, err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, toAppResp(r.Context(), s.Store, a))
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	name := chi.URLParam(r, "app")
	a, err := s.Store.GetAppByName(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "app not found"))
		return
	}
	if err := auth.RequireOwnerOrAdmin(r.Context(), s.Store, id, a.ID); err != nil {
		writeError(w, api.New(api.ErrForbidden, err.Error()))
		return
	}
	var req api.UpdateAppReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad json"))
		return
	}
	cfg := store.AppConfig{
		Port:             a.Port,
		HealthPath:       a.HealthPath,
		HealthTimeoutSec: a.HealthTimeoutSec,
		CPULimit:         a.CpuLimit,
		MemoryLimitMB:    a.MemoryLimitMb,
	}
	redeployNeeded := false
	if req.Port != nil {
		cfg.Port = *req.Port
		redeployNeeded = true
	}
	if req.HealthPath != nil {
		cfg.HealthPath = *req.HealthPath
	} // applies to next deploy only
	if req.HealthTimeoutSec != nil {
		cfg.HealthTimeoutSec = *req.HealthTimeoutSec
	} // applies to next deploy
	if req.CPULimit != nil {
		cfg.CPULimit = *req.CPULimit
		redeployNeeded = true
	}
	if req.MemoryLimitMB != nil {
		cfg.MemoryLimitMB = *req.MemoryLimitMB
		redeployNeeded = true
	}
	if err := s.Store.UpdateAppConfig(r.Context(), a.ID, cfg); err != nil {
		writeError(w, err)
		return
	}

	if req.Owners != nil {
		if !id.User.IsAdmin {
			writeError(w, api.New(api.ErrForbidden, "owner change requires admin"))
			return
		}
		existing, _ := s.Store.ListOwners(r.Context(), a.ID)
		for _, u := range existing {
			_ = s.Store.RemoveOwner(r.Context(), a.ID, u.ID)
		}
		for _, n := range *req.Owners {
			u, err := s.Store.GetUserByUsername(r.Context(), n)
			if err != nil {
				writeError(w, api.New(api.ErrInvalid, "owner: "+n))
				return
			}
			_ = s.Store.AddOwner(r.Context(), a.ID, u.ID)
		}
	}

	if redeployNeeded {
		s.maybeRedeployCurrent(r.Context(), a.ID)
	}

	a, _ = s.Store.GetAppByID(r.Context(), a.ID)
	writeJSON(w, http.StatusOK, toAppResp(r.Context(), s.Store, a))
}

func toAppResp(ctx context.Context, s *store.Store, a store.App) api.AppResp {
	owners, _ := s.ListOwners(ctx, a.ID)
	names := make([]string, 0, len(owners))
	for _, u := range owners {
		names = append(names, u.Username)
	}
	return api.AppResp{
		ID:               a.ID,
		Name:             a.Name,
		Port:             a.Port,
		HealthPath:       a.HealthPath,
		HealthTimeoutSec: a.HealthTimeoutSec,
		CPULimit:         a.CpuLimit,
		MemoryLimitMB:    a.MemoryLimitMb,
		Owners:           names,
	}
}
