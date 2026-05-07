package server

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo-apps-kit/internal/auth"
	"github.com/callmemhz/milo-apps-kit/pkg/api"
)

func (s *Server) registerRuntimeRoutes(r chi.Router) {
	r.Get("/v1/apps/{app}/status", s.handleStatus)
	r.Get("/v1/apps/{app}/logs", s.handleLogs)
	r.Post("/v1/apps/{app}/restart", s.handleRestart)
	r.Delete("/v1/apps/{app}", s.handleDeleteApp)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	a, _ := s.Store.GetAppByID(r.Context(), appID)
	out := map[string]any{"name": a.Name, "state": "down"}
	if a.CurrentDeployID != nil {
		d, _ := s.Store.GetDeployment(r.Context(), *a.CurrentDeployID)
		out["state"] = stateFromDeploy(d.Status)
		out["image_digest"] = d.ImageDigest
		if d.ContainerName != nil {
			out["container_name"] = *d.ContainerName
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func stateFromDeploy(s string) string {
	switch s {
	case "succeeded":
		return "running"
	case "deploying", "pending":
		return "deploying"
	default:
		return "failed"
	}
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	a, _ := s.Store.GetAppByID(r.Context(), appID)
	if a.CurrentDeployID == nil {
		writeError(w, api.New(api.ErrNotFound, "no current deploy"))
		return
	}
	d, _ := s.Store.GetDeployment(r.Context(), *a.CurrentDeployID)
	if d.ContainerName == nil {
		writeError(w, api.New(api.ErrNotFound, "no container"))
		return
	}
	if s.Docker == nil {
		writeError(w, api.New(api.ErrInternal, "docker not configured"))
		return
	}
	follow := r.URL.Query().Get("follow") == "true"
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}

	rdr, err := s.Docker.Logs(r.Context(), *d.ContainerName, follow, tail)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rdr.Close()
	w.Header().Set("Content-Type", "text/plain")
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := rdr.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		if !follow {
			return
		}
	}
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	if s.Deployer == nil {
		writeError(w, api.New(api.ErrInternal, "deployer not configured"))
		return
	}
	dep, err := s.Deployer.Restart(r.Context(), appID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDeploymentResp(dep))
}

func (s *Server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if err := auth.RequireAdmin(id); err != nil {
		writeError(w, api.New(api.ErrForbidden, err.Error()))
		return
	}
	name := chi.URLParam(r, "app")
	a, err := s.Store.GetAppByName(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "app not found"))
		return
	}
	if s.Deployer == nil {
		writeError(w, api.New(api.ErrInternal, "deployer not configured"))
		return
	}
	deleteVol := r.URL.Query().Get("delete_volumes") == "true"
	if err := s.Deployer.DeleteApp(r.Context(), a.ID, deleteVol); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
