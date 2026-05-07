package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo-apps-kit/internal/auth"
	"github.com/callmemhz/milo-apps-kit/internal/deploy"
	"github.com/callmemhz/milo-apps-kit/internal/store"
	"github.com/callmemhz/milo-apps-kit/pkg/api"
)

func (s *Server) registerDeploymentsRoutes(r chi.Router) {
	r.Post("/v1/apps/{app}/deployments", s.handleCreateDeployment)
	r.Get("/v1/apps/{app}/deployments", s.handleListDeployments)
	r.Get("/v1/apps/{app}/deployments/{id}", s.handleGetDeployment)
	r.Post("/v1/apps/{app}/deployments/{id}/cancel", s.handleCancelDeployment)
}

func toDeploymentResp(d store.Deployment) api.DeploymentResp {
	out := api.DeploymentResp{
		ID:          d.ID,
		AppID:       d.AppID,
		ImageDigest: d.ImageDigest,
		ImageRef:    d.ImageRef,
		Status:      d.Status,
		CreatedAt:   d.CreatedAt.UTC().Format(time.RFC3339),
	}
	if d.FailureReason != nil {
		out.FailureReason = *d.FailureReason
	}
	if d.ContainerName != nil {
		out.ContainerName = *d.ContainerName
	}
	if d.CommitSha != nil {
		out.Commit = *d.CommitSha
	}
	if d.Ref != nil {
		out.Ref = *d.Ref
	}
	if d.FinishedAt != nil {
		out.FinishedAt = d.FinishedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func errorOf(err error) api.Error {
	if e, ok := err.(*api.Error); ok {
		return *e
	}
	return api.Error{Code: api.ErrInternal, Message: err.Error()}
}

func (s *Server) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	name := chi.URLParam(r, "app")
	a, err := s.Store.GetAppByName(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "app not found"))
		return
	}
	if id.Token.Kind == "deploy" {
		if err := auth.RequireDeployScope(id, a.ID); err != nil {
			writeError(w, api.New(api.ErrForbidden, err.Error()))
			return
		}
	} else {
		if err := auth.RequireOwnerOrAdmin(r.Context(), s.Store, id, a.ID); err != nil {
			writeError(w, api.New(api.ErrForbidden, err.Error()))
			return
		}
	}
	var req api.CreateDeploymentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad json"))
		return
	}
	if req.Image == "" {
		writeError(w, api.New(api.ErrInvalid, "image required"))
		return
	}
	if s.Deployer == nil {
		writeError(w, api.New(api.ErrInternal, "deployer not configured"))
		return
	}

	dep, err := s.Deployer.Deploy(r.Context(), deploy.DeployRequest{
		AppID:       a.ID,
		AppName:     a.Name,
		ImageRef:    req.Image,
		CommitSHA:   req.Commit,
		GitRef:      req.Ref,
		TriggeredBy: id.Token.ID,
	})
	if err != nil {
		if dep.ID != 0 {
			writeJSON(w, http.StatusUnprocessableEntity, struct {
				api.Error
				Deployment api.DeploymentResp `json:"deployment"`
			}{Error: errorOf(err), Deployment: toDeploymentResp(dep)})
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDeploymentResp(dep))
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	list, _ := s.Store.ListDeploymentsForApp(r.Context(), appID, 50, 0)
	out := make([]api.DeploymentResp, 0, len(list))
	for _, d := range list {
		out = append(out, toDeploymentResp(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	did, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad id"))
		return
	}
	d, err := s.Store.GetDeployment(r.Context(), did)
	if err != nil || d.AppID != appID {
		writeError(w, api.New(api.ErrNotFound, "deploy not found"))
		return
	}
	writeJSON(w, http.StatusOK, toDeploymentResp(d))
}

func (s *Server) handleCancelDeployment(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	did, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad id"))
		return
	}
	d, err := s.Store.GetDeployment(r.Context(), did)
	if err != nil || d.AppID != appID {
		writeError(w, api.New(api.ErrNotFound, "deploy not found"))
		return
	}
	if d.Status != store.DeployDeploying {
		writeError(w, api.New(api.ErrConflict, "not in flight"))
		return
	}
	a, _ := s.Store.GetAppByID(r.Context(), appID)
	if s.Deployer != nil {
		s.Deployer.Locks().Cancel(a.Name)
	}
	w.WriteHeader(http.StatusAccepted)
}
