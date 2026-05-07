package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/callmemhz/milo-apps-kit/pkg/api"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		writeJSON(w, statusFor(apiErr.Code), apiErr)
		return
	}
	writeJSON(w, http.StatusInternalServerError, &api.Error{Code: api.ErrInternal, Message: "internal error"})
}

func statusFor(code api.ErrCode) int {
	switch code {
	case api.ErrNotFound:
		return http.StatusNotFound
	case api.ErrConflict:
		return http.StatusConflict
	case api.ErrInvalid:
		return http.StatusUnprocessableEntity
	case api.ErrUnauthorized:
		return http.StatusUnauthorized
	case api.ErrForbidden:
		return http.StatusForbidden
	case api.ErrDeployFailed:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
