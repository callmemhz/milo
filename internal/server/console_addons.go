package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/deploy"
	"github.com/callmemhz/milo/internal/docker"
	"github.com/callmemhz/milo/internal/store"
)

// consoleLoadOwnedAddon loads the {addon} route param and enforces owner/admin.
func (s *Server) consoleLoadOwnedAddon(w http.ResponseWriter, r *http.Request) (store.Addon, bool) {
	name := chi.URLParam(r, "addon")
	ad, err := s.Store.GetAddonByName(r.Context(), name)
	if err != nil {
		http.Error(w, "addon not found", http.StatusNotFound)
		return store.Addon{}, false
	}
	id, _ := auth.IdentityFromContext(r.Context())
	if id == nil || id.User == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return store.Addon{}, false
	}
	if !id.User.IsAdmin {
		ok, _ := s.Store.IsAddonOwner(r.Context(), ad.ID, id.User.ID)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return store.Addon{}, false
		}
	}
	return ad, true
}

func (s *Server) consoleAddonDetail(w http.ResponseWriter, r *http.Request) {
	ad, ok := s.consoleLoadOwnedAddon(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	id, _ := auth.IdentityFromContext(ctx)

	cname := ""
	if ad.ContainerName != nil {
		cname = *ad.ContainerName
	}
	state, uptime, mem := s.inspectCard(ctx, cname)
	if state == "down" && ad.Status != "" {
		state = ad.Status
	}

	image := ""
	if cname != "" && s.Runtime != nil {
		if info, err := s.Runtime.InspectByName(ctx, cname); err == nil {
			image = info.Image
		}
	}

	volume := "—"
	if s.Runtime != nil {
		if sz, ok := s.Runtime.VolumeSize(ctx, deploy.AddonVolumeName(ad.Name)); ok {
			volume = humanBytes(uint64(sz))
		} else {
			volume = "未使用"
		}
	}

	externalHost, externalURL := "", ""
	if ad.Exposed && s.RootDomain != "" {
		externalHost = fmt.Sprintf("%s.%s:%d", ad.Name, s.RootDomain, ad.HostPort)
		externalURL = deploy.ExternalConnectionURL(ad.Engine, ad.Name, ad.Password, s.RootDomain, ad.HostPort)
	}

	type linkRow struct{ Alias, App string }
	var links []linkRow
	if ls, err := s.Store.ListLinksForAddon(ctx, ad.ID); err == nil {
		for _, l := range ls {
			links = append(links, linkRow{Alias: l.Alias, App: l.AppName})
		}
	}

	s.render(w, "addon", map[string]any{
		"User":         id.User.Username,
		"Admin":        id.User.IsAdmin,
		"CSRF":         s.ensureCSRF(w, r),
		"Name":         ad.Name,
		"Engine":       ad.Engine,
		"Version":      ad.Version,
		"State":        state,
		"Uptime":       uptime,
		"Mem":          mem,
		"Spec":         fmt.Sprintf("%s 核 / %d MB", strconv.FormatFloat(ad.CpuLimit, 'g', -1, 64), ad.MemoryLimitMb),
		"Image":        image,
		"Volume":       volume,
		"Exposed":      ad.Exposed,
		"ExternalHost": externalHost,
		"ExternalURL":  externalURL,
		"Links":        links,
	})
}

// consoleAddonExpose toggles an addon's external access (expose/unexpose),
// reusing the same path as the API: flip the flag then re-provision.
func (s *Server) consoleAddonExpose(w http.ResponseWriter, r *http.Request) {
	ad, ok := s.consoleLoadOwnedAddon(w, r)
	if !ok {
		return
	}
	dest := "/console/addons/" + ad.Name
	if !s.checkCSRF(r) {
		http.Redirect(w, r, dest+"?err="+url.QueryEscape("会话过期，请重试"), http.StatusFound)
		return
	}
	if s.Deployer == nil {
		http.Redirect(w, r, dest+"?err="+url.QueryEscape("deployer 未配置"), http.StatusFound)
		return
	}
	want := r.FormValue("exposed") == "true"
	if ad.Exposed != want {
		if err := s.Store.SetAddonExposed(r.Context(), ad.ID, want); err != nil {
			http.Redirect(w, r, dest+"?err="+url.QueryEscape("更新失败"), http.StatusFound)
			return
		}
		if err := s.Deployer.ProvisionAddon(r.Context(), ad.ID); err != nil {
			http.Redirect(w, r, dest+"?err="+url.QueryEscape("重建容器失败: "+err.Error()), http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

func (s *Server) consoleAddonLogsStream(w http.ResponseWriter, r *http.Request) {
	ad, ok := s.consoleLoadOwnedAddon(w, r)
	if !ok {
		return
	}
	cname := ""
	if ad.ContainerName != nil {
		cname = *ad.ContainerName
	}
	s.streamContainerLogsSSE(w, r, cname)
}

func (s *Server) consoleAddonStatsStream(w http.ResponseWriter, r *http.Request) {
	ad, ok := s.consoleLoadOwnedAddon(w, r)
	if !ok {
		return
	}
	cname := ""
	if ad.ContainerName != nil {
		cname = *ad.ContainerName
	}
	s.streamContainerStatsSSE(w, r, cname)
}

// --- shared SSE helpers (used by both apps and addons) -----------------------

func (s *Server) streamContainerStatsSSE(w http.ResponseWriter, r *http.Request, name string) {
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
