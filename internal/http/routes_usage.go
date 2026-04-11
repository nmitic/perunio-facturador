package http

import (
	"net/http"

	"github.com/perunio/perunio-facturador/internal/auth"
)

// usageResponse mirrors the Node.js getDocumentUsage shape: { used, limit,
// tier, period }. limit is null for unlimited tiers.
type usageResponse struct {
	Used   int    `json:"used"`
	Limit  *int   `json:"limit"`
	Tier   string `json:"tier"`
	Period string `json:"period"`
}

// usageHandler returns the current period's facturador document count for the
// authenticated tenant.
func (s *server) usageHandler(w http.ResponseWriter, r *http.Request) {
	payload, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "Autenticación requerida")
		return
	}

	usage, err := s.pool.GetDocumentUsage(r.Context(), payload.TenantID)
	if err != nil {
		s.log.Error("get document usage", "error", err, "tenantId", payload.TenantID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno del servidor")
		return
	}

	writeSuccess(w, usageResponse{
		Used:   usage.Used,
		Limit:  usage.Limit,
		Tier:   usage.Tier,
		Period: usage.Period,
	})
}
