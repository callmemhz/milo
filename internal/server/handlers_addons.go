package server

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/deploy"
	"github.com/callmemhz/milo/internal/store"
	"github.com/callmemhz/milo/pkg/api"
)

// validLinkAlias matches env-var-prefix aliases: SCREAMING_SNAKE, leading letter.
var validLinkAlias = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,31}$`)

func (s *Server) registerAddonsRoutes(r chi.Router) {
	r.Post("/v1/addons", s.handleCreateAddon)
	r.Get("/v1/addons", s.handleListAddons)
	r.Get("/v1/addons/{addon}", s.handleGetAddon)
	r.Delete("/v1/addons/{addon}", s.handleDeleteAddon)
	r.Post("/v1/addons/{addon}/restart", s.handleRestartAddon)
	r.Post("/v1/addons/{addon}/expose", s.handleExposeAddon)
	r.Delete("/v1/addons/{addon}/expose", s.handleUnexposeAddon)
	r.Get("/v1/addons/{addon}/logs", s.handleAddonLogs)

	r.Post("/v1/apps/{app}/links", s.handleCreateLink)
	r.Get("/v1/apps/{app}/links", s.handleListLinks)
	r.Delete("/v1/apps/{app}/links/{addon}", s.handleDeleteLink)
}

// loadOwnedAddon validates the URL {addon}, ensures the caller is a user
// who owns it (or an admin), and returns the addon. Deploy tokens cannot
// touch addons.
func (s *Server) loadOwnedAddon(w http.ResponseWriter, r *http.Request) (store.Addon, bool) {
	id, _ := auth.IdentityFromContext(r.Context())
	name := chi.URLParam(r, "addon")
	addon, err := s.Store.GetAddonByName(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "addon not found"))
		return store.Addon{}, false
	}
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrForbidden, "user token required"))
		return store.Addon{}, false
	}
	if !id.User.IsAdmin {
		ok, _ := s.Store.IsAddonOwner(r.Context(), addon.ID, id.User.ID)
		if !ok {
			writeError(w, api.New(api.ErrForbidden, "not an owner"))
			return store.Addon{}, false
		}
	}
	return addon, true
}

func (s *Server) handleCreateAddon(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrUnauthorized, "no user"))
		return
	}
	var req api.CreateAddonReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad json"))
		return
	}
	if !validAppName(req.Name) {
		writeError(w, api.New(api.ErrInvalid, "invalid addon name"))
		return
	}
	_, version, err := deploy.LookupEngine(req.Engine, req.Version)
	if err != nil {
		writeError(w, api.New(api.ErrInvalid, err.Error()))
		return
	}
	if req.CPULimit == 0 {
		req.CPULimit = defaultCPULimit
	}
	if req.MemoryLimitMB == 0 {
		req.MemoryLimitMB = defaultMemMB
	}

	password, err := deploy.GeneratePassword()
	if err != nil {
		writeError(w, err)
		return
	}
	addon, err := s.Store.CreateAddon(r.Context(), req.Name, req.Engine, version, req.CPULimit, req.MemoryLimitMB, password)
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
				writeError(w, api.New(api.ErrForbidden, "non-admin can only own their own addons"))
				return
			}
		}
	}
	for _, uid := range owners {
		_ = s.Store.AddAddonOwner(r.Context(), addon.ID, uid)
	}

	// Provision synchronously: the row exists either way, so a failed
	// provision leaves status=failed and `restart` retries it.
	if s.Deployer != nil {
		if err := s.Deployer.ProvisionAddon(r.Context(), addon.ID); err != nil {
			writeError(w, err)
			return
		}
	}

	addon, _ = s.Store.GetAddonByID(r.Context(), addon.ID)
	writeJSON(w, http.StatusCreated, s.toAddonResp(r.Context(), addon, true))
}

func (s *Server) handleListAddons(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrUnauthorized, "no user"))
		return
	}
	var addons []store.Addon
	var err error
	if id.User.IsAdmin {
		addons, err = s.Store.ListAddons(r.Context())
	} else {
		addons, err = s.Store.ListAddonsByOwner(r.Context(), id.User.ID)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]api.AddonResp, 0, len(addons))
	for _, addon := range addons {
		out = append(out, s.toAddonResp(r.Context(), addon, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetAddon(w http.ResponseWriter, r *http.Request) {
	addon, ok := s.loadOwnedAddon(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.toAddonResp(r.Context(), addon, true))
}

func (s *Server) handleDeleteAddon(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if err := auth.RequireAdmin(id); err != nil {
		writeError(w, api.New(api.ErrForbidden, err.Error()))
		return
	}
	name := chi.URLParam(r, "addon")
	addon, err := s.Store.GetAddonByName(r.Context(), name)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "addon not found"))
		return
	}
	if s.Deployer == nil {
		writeError(w, api.New(api.ErrInternal, "deployer not configured"))
		return
	}

	links, err := s.Store.ListLinksForAddon(r.Context(), addon.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	force := r.URL.Query().Get("force") == "true"
	if len(links) > 0 && !force {
		apps := make([]string, 0, len(links))
		for _, l := range links {
			apps = append(apps, l.AppName)
		}
		writeError(w, &api.Error{Code: api.ErrConflict, Message: "addon has linked apps; unlink them or pass force=true", Details: apps})
		return
	}

	deleteVol := r.URL.Query().Get("delete_volumes") == "true"
	if err := s.Deployer.DeleteAddon(r.Context(), addon.ID, deleteVol); err != nil {
		writeError(w, err)
		return
	}
	// Redeploy formerly-linked apps so their containers drop the injected env
	// and the attachment to the now-removed addon network.
	for _, l := range links {
		s.maybeRedeployCurrent(r.Context(), l.AppID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestartAddon(w http.ResponseWriter, r *http.Request) {
	addon, ok := s.loadOwnedAddon(w, r)
	if !ok {
		return
	}
	if s.Deployer == nil {
		writeError(w, api.New(api.ErrInternal, "deployer not configured"))
		return
	}
	if err := s.Deployer.ProvisionAddon(r.Context(), addon.ID); err != nil {
		writeError(w, err)
		return
	}
	addon, _ = s.Store.GetAddonByID(r.Context(), addon.ID)
	writeJSON(w, http.StatusOK, s.toAddonResp(r.Context(), addon, true))
}

// handleExposeAddon turns on external access: it flips the exposed flag and
// re-provisions the addon so its port is published on the host. The addon is
// then reachable at <addon>.<root_domain>:<host_port>.
func (s *Server) handleExposeAddon(w http.ResponseWriter, r *http.Request) {
	s.setAddonExposure(w, r, true)
}

// handleUnexposeAddon turns off external access: it clears the exposed flag and
// re-provisions so the published port is dropped. The host port is retained in
// the DB so re-exposing later reuses the same port.
func (s *Server) handleUnexposeAddon(w http.ResponseWriter, r *http.Request) {
	s.setAddonExposure(w, r, false)
}

func (s *Server) setAddonExposure(w http.ResponseWriter, r *http.Request, exposed bool) {
	addon, ok := s.loadOwnedAddon(w, r)
	if !ok {
		return
	}
	if s.Deployer == nil {
		writeError(w, api.New(api.ErrInternal, "deployer not configured"))
		return
	}
	if addon.Exposed == exposed {
		// Already in the desired state; return current view without churning
		// the container.
		writeJSON(w, http.StatusOK, s.toAddonResp(r.Context(), addon, true))
		return
	}
	if err := s.Store.SetAddonExposed(r.Context(), addon.ID, exposed); err != nil {
		writeError(w, err)
		return
	}
	if err := s.Deployer.ProvisionAddon(r.Context(), addon.ID); err != nil {
		writeError(w, err)
		return
	}
	addon, _ = s.Store.GetAddonByID(r.Context(), addon.ID)
	writeJSON(w, http.StatusOK, s.toAddonResp(r.Context(), addon, true))
}

func (s *Server) handleAddonLogs(w http.ResponseWriter, r *http.Request) {
	addon, ok := s.loadOwnedAddon(w, r)
	if !ok {
		return
	}
	if addon.ContainerName == nil || *addon.ContainerName == "" {
		writeError(w, api.New(api.ErrNotFound, "no container"))
		return
	}
	s.streamLogs(w, r, *addon.ContainerName)
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	var req api.CreateLinkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad json"))
		return
	}
	addon, err := s.Store.GetAddonByName(r.Context(), req.Addon)
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "addon not found"))
		return
	}
	// Linking requires rights on both ends: the app (checked above) and the
	// addon — otherwise any app owner could read someone else's database
	// credentials by linking to it.
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		writeError(w, api.New(api.ErrForbidden, "user token required"))
		return
	}
	if !id.User.IsAdmin {
		isOwner, _ := s.Store.IsAddonOwner(r.Context(), addon.ID, id.User.ID)
		if !isOwner {
			writeError(w, api.New(api.ErrForbidden, "not an owner of addon"))
			return
		}
	}
	if req.Alias != "" && !validLinkAlias.MatchString(req.Alias) {
		writeError(w, api.New(api.ErrInvalid, "invalid alias (want e.g. CACHE, PRIMARY_DB)"))
		return
	}
	envKey := deploy.LinkEnvKey(addon.Engine, req.Alias)
	if envKey == "PORT" {
		writeError(w, api.New(api.ErrInvalid, "alias collides with platform env"))
		return
	}
	existing, err := s.Store.ListLinksForApp(r.Context(), appID)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, l := range existing {
		if deploy.LinkEnvKey(l.Engine, l.Alias) == envKey {
			writeError(w, api.New(api.ErrConflict, "env key "+envKey+" already used by link to "+l.AddonName+"; pick an alias"))
			return
		}
	}

	link, err := s.Store.CreateLink(r.Context(), appID, addon.ID, req.Alias)
	if err != nil {
		writeError(w, api.New(api.ErrConflict, "app is already linked to this addon"))
		return
	}
	s.maybeRedeployCurrent(r.Context(), appID)

	a, _ := s.Store.GetAppByID(r.Context(), appID)
	writeJSON(w, http.StatusCreated, api.LinkResp{
		App:    a.Name,
		Addon:  addon.Name,
		Engine: addon.Engine,
		Alias:  link.Alias,
		EnvKey: envKey,
	})
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	links, err := s.Store.ListLinksForApp(r.Context(), appID)
	if err != nil {
		writeError(w, err)
		return
	}
	a, _ := s.Store.GetAppByID(r.Context(), appID)
	out := make([]api.LinkResp, 0, len(links))
	for _, l := range links {
		out = append(out, api.LinkResp{
			App:    a.Name,
			Addon:  l.AddonName,
			Engine: l.Engine,
			Alias:  l.Alias,
			EnvKey: deploy.LinkEnvKey(l.Engine, l.Alias),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	addon, err := s.Store.GetAddonByName(r.Context(), chi.URLParam(r, "addon"))
	if err != nil {
		writeError(w, api.New(api.ErrNotFound, "addon not found"))
		return
	}
	if _, err := s.Store.GetLink(r.Context(), appID, addon.ID); err != nil {
		writeError(w, api.New(api.ErrNotFound, "link not found"))
		return
	}
	if err := s.Store.DeleteLink(r.Context(), appID, addon.ID); err != nil {
		writeError(w, err)
		return
	}
	s.maybeRedeployCurrent(r.Context(), appID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) toAddonResp(ctx context.Context, addon store.Addon, includeURL bool) api.AddonResp {
	owners, _ := s.Store.ListAddonOwners(ctx, addon.ID)
	names := make([]string, 0, len(owners))
	for _, u := range owners {
		names = append(names, u.Username)
	}
	links, _ := s.Store.ListLinksForAddon(ctx, addon.ID)
	apps := make([]string, 0, len(links))
	for _, l := range links {
		apps = append(apps, l.AppName)
	}
	resp := api.AddonResp{
		ID:            addon.ID,
		Name:          addon.Name,
		Engine:        addon.Engine,
		Version:       addon.Version,
		Status:        addon.Status,
		CPULimit:      addon.CpuLimit,
		MemoryLimitMB: addon.MemoryLimitMb,
		Owners:        names,
		LinkedApps:    apps,
		Exposed:       addon.Exposed,
		HostPort:      int(addon.HostPort),
	}
	if includeURL {
		resp.URL = deploy.ConnectionURL(addon.Engine, addon.Name, addon.Password)
		if addon.Exposed {
			resp.ExternalURL = deploy.ExternalConnectionURL(addon.Engine, addon.Name, addon.Password, s.RootDomain, addon.HostPort)
		}
	}
	return resp
}
