package conversion

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Handler struct {
	service *Service
	enabled bool
}

func NewHandler(service *Service, enabled bool) *Handler {
	return &Handler{service: service, enabled: enabled}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/conversions", h.convert)
}

func (h *Handler) convert(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.enabled {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "worker conversion is disabled"})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid conversion request"})
		return
	}
	result, err := h.service.Convert(r.Context(), request)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("conversion refused: %v", err)})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
