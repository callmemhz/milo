package server

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/callmemhz/milo/internal/auth"
)

// parseMiloVolume derives the milo resource (kind, instance) a volume belongs to
// from its name. Returns empty kind for non-milo volumes.
func parseMiloVolume(name string) (kind, instance string) {
	switch {
	case strings.HasPrefix(name, "milo-app-") && strings.HasSuffix(name, "-data"):
		return "app", strings.TrimSuffix(strings.TrimPrefix(name, "milo-app-"), "-data")
	case strings.HasPrefix(name, "milo-addon-") && strings.HasSuffix(name, "-data"):
		return "addon", strings.TrimSuffix(strings.TrimPrefix(name, "milo-addon-"), "-data")
	}
	return "", ""
}

type volRow struct {
	Name     string
	Kind     string // app / addon / 其他
	Instance string // owning app/addon name (milo volumes)
	Size     string
	InUse    bool
	Orphan   bool // milo volume whose owning app/addon no longer exists
}

func (s *Server) consoleVolumes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := auth.IdentityFromContext(ctx)

	lang := langFromRequest(r)
	var rows []volRow
	if s.Runtime != nil {
		vols, _ := s.Runtime.VolumeList(ctx)
		for _, v := range vols {
			row := volRow{Name: v.Name, Kind: translate(lang, "vol.kind_other"), InUse: v.RefCount > 0}
			if v.Size >= 0 {
				row.Size = humanBytes(uint64(v.Size))
			} else {
				row.Size = "—"
			}
			if kind, inst := parseMiloVolume(v.Name); kind != "" {
				row.Kind = kind
				row.Instance = inst
				var err error
				if kind == "app" {
					_, err = s.Store.GetAppByName(ctx, inst)
				} else {
					_, err = s.Store.GetAddonByName(ctx, inst)
				}
				row.Orphan = err != nil // owner gone -> orphaned data volume
			}
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool {
			// milo volumes (those with a parsed owning instance) first, then by name.
			mi, mj := rows[i].Instance != "", rows[j].Instance != ""
			if mi != mj {
				return mi
			}
			return rows[i].Name < rows[j].Name
		})
	}
	s.render(w, r, "volumes", map[string]any{
		"User":    id.User.Username,
		"Admin":   true,
		"CSRF":    s.ensureCSRF(w, r),
		"Volumes": rows,
		"Flash":   r.URL.Query().Get("msg"),
		"Error":   r.URL.Query().Get("err"),
	})
}

func (s *Server) consoleVolumeDelete(w http.ResponseWriter, r *http.Request) {
	lang := langFromRequest(r)
	flash := func(key, msg string) {
		http.Redirect(w, r, "/console/admin/volumes?"+key+"="+url.QueryEscape(msg), http.StatusFound)
	}
	if !s.checkCSRF(r) {
		flash("err", translate(lang, "login.expired"))
		return
	}
	if err := r.ParseForm(); err != nil {
		flash("err", translate(lang, "m.form_parse"))
		return
	}
	names := r.Form["names"]
	force := r.FormValue("force") == "on"
	if s.Runtime == nil || len(names) == 0 {
		flash("err", translate(lang, "m.no_vol_sel"))
		return
	}
	ok, failed := 0, 0
	for _, n := range names {
		if n == "" {
			continue
		}
		if err := s.Runtime.VolumeRemove(r.Context(), n, force); err != nil {
			failed++
		} else {
			ok++
		}
	}
	msg := fmt.Sprintf(translate(lang, "m.vol_deleted"), ok)
	if failed > 0 {
		msg += fmt.Sprintf(translate(lang, "m.vol_failed"), failed)
	}
	flash("msg", msg)
}
