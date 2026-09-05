// Package httpx assembles the REST/SSE/WebSocket HTTP surface: routing,
// security middleware and domain handlers. Business rules stay in the domain
// packages; this layer only translates between HTTP and services.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
)

// APIError is the stable error envelope.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// writeJSON writes a success envelope.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		_, _ = w.Write([]byte(`{"data":null}`))
		return
	}
	enc := json.NewEncoder(w)
	_ = enc.Encode(map[string]any{"data": v})
}

// writeErr writes an error envelope with an inferred status code.
func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": APIError{Code: code, Message: msg}})
}

// decodeJSON strictly decodes a JSON body with size cap.
func decodeJSON(r *http.Request, dst any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 4 << 20
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

// codeStatus maps domain error codes to HTTP status.
func codeStatus(code string) int {
	switch code {
	case "unauthenticated":
		return http.StatusUnauthorized
	case "forbidden":
		return http.StatusForbidden
	case "not_found":
		return http.StatusNotFound
	case "already_setup", "conflict", "host_key_changed", "modified", "in_use", "is_jump_host":
		return http.StatusConflict
	case "jump_cycle", "jump_too_deep", "empty_targets", "bad_request":
		return http.StatusUnprocessableEntity
	case "rate_limited":
		return http.StatusTooManyRequests
	case "too_large":
		return http.StatusRequestEntityTooLarge
	case "conn_timeout":
		return http.StatusGatewayTimeout
	case "conn_lost":
		return http.StatusBadGateway
	default:
		return http.StatusBadRequest
	}
}

// fail maps a Go error to an envelope using heuristics.
func fail(w http.ResponseWriter, err error) {
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		writeErr(w, codeStatus(coded.Code()), coded.Code(), err.Error())
		return
	}
	switch {
	case errors.Is(err, errUnauthenticated):
		writeErr(w, 401, "unauthenticated", err.Error())
	case errors.Is(err, errForbidden):
		writeErr(w, 403, "forbidden", err.Error())
	default:
		writeErr(w, 400, "bad_request", err.Error())
	}
}

var (
	errUnauthenticated = errors.New("unauthenticated")
	errForbidden       = errors.New("forbidden")
)
