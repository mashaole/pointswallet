package controller

import (
	"net/http"

	"pointswallet/internal/models"
	"pointswallet/internal/models/dto"
	authsvc "pointswallet/internal/service/auth"
)

type AuthController struct {
	auth     *authsvc.Service
	maxBytes int64
}

func NewAuthController(auth *authsvc.Service, maxBytes int64) *AuthController {
	return &AuthController{auth: auth, maxBytes: maxBytes}
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.LoginRequest) { d.Sanitize() }, func(d *dto.LoginRequest) error { return d.Validate() }) {
		return
	}
	result, err := c.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"access_token": result.AccessToken,
		"account_id":   result.AccountID,
		"role":         result.Role,
	})
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		WriteError(w, MapDomainError(models.ErrUnauthorized))
		return
	}
	if err := c.auth.Logout(r.Context(), claims.JTI); err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *AuthController) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ForgotPasswordRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.ForgotPasswordRequest) { d.Sanitize() }, func(d *dto.ForgotPasswordRequest) error { return d.Validate() }) {
		return
	}
	result, _ := c.auth.ForgotPassword(r.Context(), req.Email)
	writeData(w, http.StatusOK, map[string]any{
		"message":     result.Message,
		"reset_token": result.ResetToken,
	})
}

func (c *AuthController) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetPasswordRequest
	if !decodeAndValidateJSON(w, r, c.maxBytes, &req, func(d *dto.ResetPasswordRequest) { d.Sanitize() }, func(d *dto.ResetPasswordRequest) error { return d.Validate() }) {
		return
	}
	if err := c.auth.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		WriteError(w, MapDomainError(err))
		return
	}
	writeData(w, http.StatusOK, map[string]any{"message": "Password updated"})
}
