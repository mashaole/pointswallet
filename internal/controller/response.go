package controller

import (
	"encoding/json"
	"errors"
	"net/http"

	"pointswallet/internal/models"
)

type errorResponse struct {
	Error apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"status"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

func WriteError(w http.ResponseWriter, apiErr models.APIError) {
	writeError(w, apiErr)
}

func writeError(w http.ResponseWriter, apiErr models.APIError) {
	writeJSON(w, apiErr.Status, errorResponse{Error: apiErrorBody{
		Code: apiErr.Code, Message: apiErr.Message, Status: apiErr.Status,
	}})
}

func MapDomainError(err error) models.APIError {
	if err == nil {
		return models.NewAPIError("internal_error", "unexpected nil error", http.StatusInternalServerError)
	}
	var apiErr models.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	switch {
	case errors.Is(err, models.ErrInsufficientBalance):
		return models.NewAPIError("insufficient_balance", "Spend would exceed available balance", http.StatusUnprocessableEntity)
	case errors.Is(err, models.ErrDuplicateRef):
		return models.NewAPIError("duplicate_ref", "Transaction ref already exists", http.StatusConflict)
	case errors.Is(err, models.ErrEmailAlreadyExists):
		return models.NewAPIError("email_already_exists", "Email already registered", http.StatusConflict)
	case errors.Is(err, models.ErrInvalidRole):
		return models.NewAPIError("invalid_role", "Role must be member or admin", http.StatusBadRequest)
	case errors.Is(err, models.ErrUnauthorized):
		return models.NewAPIError("unauthorized", "Unauthorized", http.StatusUnauthorized)
	case errors.Is(err, models.ErrForbidden):
		return models.NewAPIError("forbidden", "Forbidden", http.StatusForbidden)
	case errors.Is(err, models.ErrNotFound):
		return models.NewAPIError("not_found", "Resource not found", http.StatusNotFound)
	case errors.Is(err, models.ErrValidation), errors.Is(err, models.ErrInvalidEmail),
		errors.Is(err, models.ErrInvalidKind), errors.Is(err, models.ErrInvalidPoints):
		return models.NewAPIError("validation_error", err.Error(), http.StatusBadRequest)
	default:
		return models.NewAPIError("internal_error", "Internal server error", http.StatusInternalServerError)
	}
}
