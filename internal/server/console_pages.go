package server

import (
	"bufio"
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/store"
)

// buildAppCards builds table rows for the given apps; withOwners fills the owner
// column (admin views only).
func (s *Server) buildAppCards(ctx context.Context, apps []store.App, withOwners bool) []appCard {
	out := make([]appCard, 0, len(apps))
	for _, a := range apps {
		state, uptime, mem := s.inspectCard(ctx, s.currentContainer(ctx, a))
		c := appCard{Name: a.Name, State: state, Uptime: uptime, Mem: mem}
		if withOwners {
			owners, _ := s.Store.ListOwners(ctx, a.ID)
			c.Owners = ownerNames(owners)
		}
		out = append(out, c)
	}
	return out
}

func (s *Server) buildAddonCards(ctx context.Context, addons []store.Addon, withOwners bool) []addonCard {
	out := make([]addonCard, 0, len(addons))
	for _, ad := range addons {
		name := ""
		if ad.ContainerName != nil {
			name = *ad.ContainerName
		}
		state, uptime, mem := s.inspectCard(ctx, name)
		if state == "down" && ad.Status != "" {
			state = ad.Status
		}
		c := addonCard{
			Name: ad.Name, Engine: ad.Engine, Status: state,
			Uptime: uptime, Mem: mem, Exposed: ad.Exposed,
		}
		if withOwners {
			owners, _ := s.Store.ListAddonOwners(ctx, ad.ID)
			c.Owners = ownerNames(owners)
		}
		out = append(out, c)
	}
	return out
}

// consoleDashboard shows the signed-in user's OWN apps and addons (admins see
// their personal resources here; the org-wide view is /console/admin/instances).
func (s *Server) consoleDashboard(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	ctx := r.Context()

	apps, _ := s.Store.ListAppsByOwner(ctx, id.User.ID)
	addons, _ := s.Store.ListAddonsByOwner(ctx, id.User.ID)

	s.render(w, r, "dashboard", map[string]any{
		"User":   id.User.Username,
		"Admin":  id.User.IsAdmin,
		"CSRF":   s.ensureCSRF(w, r),
		"Apps":   s.buildAppCards(ctx, apps, false),
		"Addons": s.buildAddonCards(ctx, addons, false),
	})
}

// consoleAllInstances is the admin org-wide view of every app and addon.
func (s *Server) consoleAllInstances(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := auth.IdentityFromContext(ctx)
	apps, _ := s.Store.ListApps(ctx)
	addons, _ := s.Store.ListAddons(ctx)

	s.render(w, r, "instances", map[string]any{
		"User":   id.User.Username,
		"Admin":  true,
		"CSRF":   s.ensureCSRF(w, r),
		"Apps":   s.buildAppCards(ctx, apps, true),
		"Addons": s.buildAddonCards(ctx, addons, true),
	})
}

func (s *Server) consoleAppDetail(w http.ResponseWriter, r *http.Request) {
	a, ok := s.consoleLoadOwnedApp(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	id, _ := auth.IdentityFromContext(ctx)

	state, uptime, _ := s.inspectCard(ctx, s.currentContainer(ctx, a))
	var image, ref string
	if a.CurrentDeployID != nil {
		if d, err := s.Store.GetDeployment(ctx, *a.CurrentDeployID); err == nil {
			image = d.ImageRef
			if d.Ref != nil {
				ref = *d.Ref
			}
		}
	}

	// Public address: apps are always fronted by Caddy at <name>.<root_domain>.
	address := ""
	if s.RootDomain != "" {
		address = "https://" + a.Name + "." + s.RootDomain
	}

	lang := langFromRequest(r)
	// Data volume usage.
	volName := fmt.Sprintf("milo-app-%s-data", a.Name)
	volume := "—"
	if s.Runtime != nil {
		if sz, ok := s.Runtime.VolumeSize(ctx, volName); ok {
			volume = humanBytes(uint64(sz))
		} else {
			volume = translate(lang, "f.unused")
		}
	}

	// Linked add-ons.
	type linkRow struct{ Alias, Addon, Engine string }
	var links []linkRow
	if ls, err := s.Store.ListLinksForApp(ctx, a.ID); err == nil {
		for _, l := range ls {
			links = append(links, linkRow{Alias: l.Alias, Addon: l.AddonName, Engine: l.Engine})
		}
	}

	s.render(w, r, "app", map[string]any{
		"User":    id.User.Username,
		"Admin":   id.User.IsAdmin,
		"CSRF":    s.ensureCSRF(w, r),
		"Name":    a.Name,
		"State":   state,
		"Uptime":  uptime,
		"Port":    a.Port,
		"Spec":    fmt.Sprintf("%s %s / %d MB", strconv.FormatFloat(a.CpuLimit, 'g', -1, 64), translate(lang, "unit.cores"), a.MemoryLimitMb),
		"Image":   image,
		"Ref":     ref,
		"Address": address,
		"Volume":  volume,
		"Links":   links,
	})
}

// consoleLoadOwnedApp loads the {app} route param and enforces owner/admin.
// On failure it writes an HTML error and returns ok=false.
func (s *Server) consoleLoadOwnedApp(w http.ResponseWriter, r *http.Request) (store.App, bool) {
	name := chi.URLParam(r, "app")
	a, err := s.Store.GetAppByName(r.Context(), name)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return store.App{}, false
	}
	id, _ := auth.IdentityFromContext(r.Context())
	if err := auth.RequireOwnerOrAdmin(r.Context(), s.Store, id, a.ID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return store.App{}, false
	}
	return a, true
}

func (s *Server) consoleAppLogsStream(w http.ResponseWriter, r *http.Request) {
	a, ok := s.consoleLoadOwnedApp(w, r)
	if !ok {
		return
	}
	s.streamContainerLogsSSE(w, r, s.currentContainer(r.Context(), a))
}

func (s *Server) consoleAppStatsStream(w http.ResponseWriter, r *http.Request) {
	a, ok := s.consoleLoadOwnedApp(w, r)
	if !ok {
		return
	}
	s.streamContainerStatsSSE(w, r, s.currentContainer(r.Context(), a))
}

// streamContainerLogsSSE streams a container's logs as SSE `message` events,
// one per demultiplexed log line. Shared by app and addon log views.
func (s *Server) streamContainerLogsSSE(w http.ResponseWriter, r *http.Request, name string) {
	flusher, sseOK := w.(http.Flusher)
	if !sseOK {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	sseHeaders(w)
	lang := langFromRequest(r)
	if name == "" || s.Docker == nil {
		sseEvent(w, flusher, "message", translate(lang, "sse.no_ctr"))
		return
	}
	rc, err := s.Docker.Logs(r.Context(), name, true, "200")
	if err != nil {
		sseEvent(w, flusher, "message", fmt.Sprintf(translate(lang, "sse.log_error"), err.Error()))
		return
	}
	defer rc.Close()
	go func() { <-r.Context().Done(); rc.Close() }()

	// Demultiplex docker's stdout/stderr framing into clean lines via a pipe.
	pr, pw := io.Pipe()
	go func() { _, _ = stdcopy.StdCopy(pw, pw, rc); pw.Close() }()

	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		sseEvent(w, flusher, "message", sc.Text())
	}
}

// --- SSE plumbing ------------------------------------------------------------

func sseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// sseEvent writes one SSE event. For htmx the payload is HTML; callers that emit
// raw log text must escape it themselves (logs use the "message" event and are
// HTML-escaped here to be safe).
func sseEvent(w http.ResponseWriter, f http.Flusher, event, data string) {
	if event == "message" {
		data = template.HTMLEscapeString(data)
	}
	// SSE requires each line of data prefixed with "data:"; collapse newlines.
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	f.Flush()
}
