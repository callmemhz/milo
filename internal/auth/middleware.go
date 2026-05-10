package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/callmemhz/milo/internal/store"
	"github.com/callmemhz/milo/pkg/api"
)

type ctxKey int

const identityKey ctxKey = 1

type Identity struct {
	Token store.Token
	User  *store.User // populated when Token.Kind == "user"
	AppID *int64      // populated when Token.Kind == "deploy"
}

func IdentityFromContext(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(identityKey).(*Identity)
	return id, ok
}

func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

type Authenticator struct {
	Store *store.Store
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeAPIError(w, http.StatusUnauthorized, api.ErrUnauthorized, "missing bearer token")
			return
		}
		plaintext := strings.TrimPrefix(h, "Bearer ")
		tk, err := a.Store.GetTokenByHash(r.Context(), Hash(plaintext))
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, api.ErrUnauthorized, "invalid token")
			return
		}
		id := &Identity{Token: tk}
		switch tk.Kind {
		case "user":
			if tk.UserID == nil {
				writeAPIError(w, http.StatusUnauthorized, api.ErrUnauthorized, "malformed token")
				return
			}
			u, err := a.Store.GetUserByID(r.Context(), *tk.UserID)
			if err != nil {
				writeAPIError(w, http.StatusUnauthorized, api.ErrUnauthorized, "user gone")
				return
			}
			id.User = &u
		case "deploy":
			if tk.AppID == nil {
				writeAPIError(w, http.StatusUnauthorized, api.ErrUnauthorized, "malformed token")
				return
			}
			id.AppID = tk.AppID
		default:
			writeAPIError(w, http.StatusUnauthorized, api.ErrUnauthorized, "unknown kind")
			return
		}
		_ = a.Store.TouchTokenLastUsed(r.Context(), tk.ID)
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

// writeAPIError emits a minimal JSON error. Duplicated in package server's errors.go;
// kept here to avoid an import cycle (server depends on auth).
func writeAPIError(w http.ResponseWriter, status int, code api.ErrCode, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"code":"` + string(code) + `","message":"` + msg + `"}`))
}
