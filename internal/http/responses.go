package http

import (
	"encoding/json"
	"net/http"
)

// successResponse is the JSON envelope for successful responses, matching the
// shape produced by perunio-backend so the existing frontend can consume both
// services without per-call shape detection.
type successResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
}

// errorResponse mirrors the Node.js error envelope: { success, error, code }.
type errorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code"`
}

// writeSuccess sends a 200 with the given payload wrapped in the success envelope.
func writeSuccess(w http.ResponseWriter, data any) {
	writeSuccessStatus(w, http.StatusOK, data)
}

// writeSuccessStatus sends an arbitrary status with the success envelope. Use
// for 201 Created etc.
func writeSuccessStatus(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(successResponse{Success: true, Data: data})
}

// writeError sends a JSON error response in the shared envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Success: false, Error: message, Code: code})
}

// decodeBody parses a JSON request body into v. Returns a 400-friendly error
// for malformed input.
func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
