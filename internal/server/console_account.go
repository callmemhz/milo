package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/internal/store"
)

// consoleBaseURL reconstructs the public base URL the client is using, so the
// CLI login command we print points at this same milod.
func consoleBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

type tokenRow struct {
	ID       int64
	Name     string
	Created  string
	LastUsed string
	Revoked  bool
}

func (s *Server) tokenRows(r *http.Request, userID int64) []tokenRow {
	lang := langFromRequest(r)
	toks, _ := s.Store.ListUserTokens(r.Context(), userID)
	rows := make([]tokenRow, 0, len(toks))
	for _, t := range toks {
		row := tokenRow{ID: t.ID, Created: t.CreatedAt.Format("2006-01-02"), Revoked: t.RevokedAt != nil}
		if t.Name != nil {
			row.Name = *t.Name
		}
		if t.LastUsedAt != nil {
			row.LastUsed = t.LastUsedAt.Format("2006-01-02 15:04")
		} else {
			row.LastUsed = translate(lang, "acct.never")
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *Server) consoleAccount(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	s.renderAccount(w, r, id.User, "", "")
}

// renderAccount renders the self-service account page. newToken (plaintext) is
// non-empty only right after creation and is shown once.
func (s *Server) renderAccount(w http.ResponseWriter, r *http.Request, u *store.User, newToken, loginCmd string) {
	s.render(w, r, "account", map[string]any{
		"User":     u.Username,
		"Admin":    u.IsAdmin,
		"CSRF":     s.ensureCSRF(w, r),
		"Tokens":   s.tokenRows(r, u.ID),
		"NewToken": newToken,
		"LoginCmd": loginCmd,
	})
}

func (s *Server) consoleAccountTokenCreate(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if !s.checkCSRF(r) {
		s.renderAccount(w, r, id.User, "", "")
		return
	}
	plaintext, cmd, ok := s.issueToken(r, id.User.ID, r.FormValue("name"))
	if !ok {
		s.renderAccount(w, r, id.User, "", "")
		return
	}
	s.renderAccount(w, r, id.User, plaintext, cmd)
}

func (s *Server) consoleAccountTokenRevoke(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	if s.checkCSRF(r) {
		s.revokeOwnedToken(r, id.User.ID, r.FormValue("id"))
	}
	http.Redirect(w, r, "/console/account", http.StatusFound)
}

// consoleUserToken lets an admin (or the user themselves) issue a CLI token for
// a given user from the users page, showing the plaintext once.
func (s *Server) consoleUserToken(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.IdentityFromContext(r.Context())
	lang := langFromRequest(r)
	if !s.checkCSRF(r) {
		usersFlash(w, r, "err", "login.expired")
		return
	}
	username := r.FormValue("username")
	if !id.User.IsAdmin && id.User.Username != username {
		usersFlash(w, r, "err", "m.self_freeze") // reuse "not allowed" style msg
		return
	}
	target, err := s.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		usersFlash(w, r, "err", "m.user_missing")
		return
	}
	plaintext, cmd, ok := s.issueToken(r, target.ID, "console-issued")
	if !ok {
		usersFlash(w, r, "err", "m.op_failed")
		return
	}
	s.render(w, r, "token_result", map[string]any{
		"User":     id.User.Username,
		"Admin":    id.User.IsAdmin,
		"CSRF":     s.ensureCSRF(w, r),
		"ForUser":  fmt.Sprintf(translate(lang, "acct.for_user"), username),
		"NewToken": plaintext,
		"LoginCmd": cmd,
	})
}

// issueToken creates a user token and returns its plaintext + a ready CLI login
// command. The plaintext is never stored or logged.
func (s *Server) issueToken(r *http.Request, userID int64, name string) (plaintext, loginCmd string, ok bool) {
	plaintext, err := auth.Generate()
	if err != nil {
		return "", "", false
	}
	if _, err := s.Store.CreateUserToken(r.Context(), userID, auth.Hash(plaintext), name); err != nil {
		return "", "", false
	}
	loginCmd = fmt.Sprintf("milo auth login --endpoint %s --token %s", consoleBaseURL(r), plaintext)
	return plaintext, loginCmd, true
}

// revokeOwnedToken revokes a token only if it belongs to userID.
func (s *Server) revokeOwnedToken(r *http.Request, userID int64, idStr string) {
	tid, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}
	toks, _ := s.Store.ListUserTokens(r.Context(), userID)
	for _, t := range toks {
		if t.ID == tid {
			_ = s.Store.RevokeToken(r.Context(), tid)
			return
		}
	}
}
