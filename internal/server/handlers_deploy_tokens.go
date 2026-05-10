package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/callmemhz/milo/internal/auth"
	"github.com/callmemhz/milo/pkg/api"
)

func (s *Server) registerAppTokensRoutes(r chi.Router) {
	r.Post("/v1/apps/{app}/tokens", s.handleCreateAppToken)
	r.Get("/v1/apps/{app}/tokens", s.handleListAppTokens)
	r.Delete("/v1/apps/{app}/tokens/{id}", s.handleRevokeAppToken)
}

func (s *Server) handleCreateAppToken(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	var req api.CreateTokenReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	plaintext, err := auth.Generate()
	if err != nil {
		writeError(w, err)
		return
	}
	tk, err := s.Store.CreateDeployToken(r.Context(), appID, auth.Hash(plaintext), req.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.CreateTokenResp{ID: tk.ID, Token: plaintext, Name: req.Name})
}

func (s *Server) handleListAppTokens(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	tokens, _ := s.Store.ListDeployTokens(r.Context(), appID)
	out := make([]api.TokenResp, 0, len(tokens))
	for _, tk := range tokens {
		nm := ""
		if tk.Name != nil {
			nm = *tk.Name
		}
		out = append(out, api.TokenResp{ID: tk.ID, Name: nm, Kind: tk.Kind})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRevokeAppToken(w http.ResponseWriter, r *http.Request) {
	_, ok := s.loadOwnedApp(w, r)
	if !ok {
		return
	}
	tokID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, api.New(api.ErrInvalid, "bad id"))
		return
	}
	if err := s.Store.RevokeToken(r.Context(), tokID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
