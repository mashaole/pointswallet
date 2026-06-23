package controller

import (
	"encoding/json"
	"io"
	"net/http"

	"pointswallet/internal/models"
)

func decodeAndValidateJSON[T any](
	w http.ResponseWriter,
	r *http.Request,
	maxBytes int64,
	dst *T,
	sanitize func(*T),
	validate func() error,
) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, models.NewAPIError("validation_error", "Invalid JSON body", http.StatusBadRequest))
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, models.NewAPIError("validation_error", "Request body must contain a single JSON object", http.StatusBadRequest))
		return false
	}
	if sanitize != nil {
		sanitize(dst)
	}
	if validate != nil {
		if err := validate(); err != nil {
			writeError(w, MapDomainError(err))
			return false
		}
	}
	return true
}
