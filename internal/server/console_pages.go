package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/docker"
	"github.com/callmemhz/milo/internal/store"
)

func (s *Server) consoleDashboard(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	ctx := r.Context()

	var apps []store.App
	var addons []store.Addon
	if id.User.IsAdmin {
		apps, _ = s.Store.ListApps(ctx)
		addons, _ = s.Store.ListAddons(ctx)
	} else {
		apps, _ = s.Store.ListAppsByOwner(ctx, id.User.ID)
		addons, _ = s.Store.ListAddonsByOwner(ctx, id.User.ID)
	}

	appCards := make([]appCard, 0, len(apps))
	for _, a := range apps {
		state, uptime, mem := s.inspectCard(ctx, s.currentContainer(ctx, a))
		c := appCard{Name: a.Name, State: state, Uptime: uptime, Mem: mem}
		if id.User.IsAdmin {
			owners, _ := s.Store.ListOwners(ctx, a.ID)
			c.Owners = ownerNames(owners)
		}
		appCards = append(appCards, c)
	}
	addonCards := make([]addonCard, 0, len(addons))
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
		if id.User.IsAdmin {
			owners, _ := s.Store.ListAddonOwners(ctx, ad.ID)
			c.Owners = ownerNames(owners)
		}
		addonCards = append(addonCards, c)
	}

	s.render(w, "dashboard", map[string]any{
		"User":   id.User.Username,
		"Admin":  id.User.IsAdmin,
		"CSRF":   s.ensureCSRF(w, r),
		"Apps":   appCards,
		"Addons": addonCards,
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

	s.render(w, "app", map[string]any{
		"User":   id.User.Username,
		"Admin":  id.User.IsAdmin,
		"CSRF":   s.ensureCSRF(w, r),
		"Name":   a.Name,
		"State":  state,
		"Uptime": uptime,
		"Port":   a.Port,
		"Image":  image,
		"Ref":    ref,
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

// consoleAppLogsStream streams the app's container logs as Server-Sent Events.
// Each demultiplexed log line becomes one `message` event.
func (s *Server) consoleAppLogsStream(w http.ResponseWriter, r *http.Request) {
	a, ok := s.consoleLoadOwnedApp(w, r)
	if !ok {
		return
	}
	name := s.currentContainer(r.Context(), a)
	flusher, sseOK := w.(http.Flusher)
	if !sseOK {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	sseHeaders(w)
	if name == "" || s.Docker == nil {
		sseEvent(w, flusher, "message", "（暂无运行中的容器）")
		return
	}

	rc, err := s.Docker.Logs(r.Context(), name, true, "200")
	if err != nil {
		sseEvent(w, flusher, "message", "无法读取日志: "+err.Error())
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

// consoleAppStatsStream streams live CPU/memory as SSE `stats` events carrying a
// small HTML fragment that htmx swaps into the overview.
func (s *Server) consoleAppStatsStream(w http.ResponseWriter, r *http.Request) {
	a, ok := s.consoleLoadOwnedApp(w, r)
	if !ok {
		return
	}
	name := s.currentContainer(r.Context(), a)
	flusher, sseOK := w.(http.Flusher)
	if !sseOK {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	sseHeaders(w)
	if name == "" || s.Runtime == nil {
		sseEvent(w, flusher, "stats", `<span class="muted">无负载数据</span>`)
		return
	}

	rc, err := s.Runtime.StatsStream(r.Context(), name)
	if err != nil {
		sseEvent(w, flusher, "stats", `<span class="muted">无负载数据</span>`)
		return
	}
	defer rc.Close()
	go func() { <-r.Context().Done(); rc.Close() }()

	dec := json.NewDecoder(rc)
	for {
		var frame container.StatsResponse
		if err := dec.Decode(&frame); err != nil {
			return
		}
		st := docker.ParseStats(frame)
		frag := fmt.Sprintf(
			`<span class="metric">CPU <b>%.1f%%</b></span><span class="metric">内存 <b>%s</b></span>`,
			st.CPUPercent, template.HTMLEscapeString(humanBytes(st.MemoryUsage)))
		sseEvent(w, flusher, "stats", frag)
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
