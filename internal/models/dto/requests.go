package dto

import (
	"strings"
	"time"

	"pointswallet/internal/models"
)

type CreateAccountRequest struct {
	AccountID string `json:"account_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Role      string `json:"role"`
}

func (r *CreateAccountRequest) Sanitize() {
	r.AccountID = strings.TrimSpace(r.AccountID)
	r.Name = strings.TrimSpace(r.Name)
	r.Password = strings.TrimSpace(r.Password)
	r.Role = strings.ToLower(strings.TrimSpace(r.Role))
	if e, err := models.NormalizeEmail(r.Email); err == nil {
		r.Email = e
	} else {
		r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	}
	if r.Role == "" {
		r.Role = models.RoleMember
	}
}

func (r CreateAccountRequest) Validate() error {
	if r.AccountID == "" {
		return models.ErrFieldRequired("account_id")
	}
	if r.Name == "" {
		return models.ErrFieldRequired("name")
	}
	if err := models.ValidateEmail(r.Email); err != nil {
		return err
	}
	if r.Password == "" {
		return models.ErrFieldRequired("password")
	}
	if r.Role != models.RoleMember && r.Role != models.RoleAdmin {
		return models.ErrInvalidRole
	}
	return nil
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r *LoginRequest) Sanitize() {
	r.Email = strings.TrimSpace(r.Email)
	r.Password = strings.TrimSpace(r.Password)
}

func (r LoginRequest) Validate() error {
	if err := models.ValidateEmail(r.Email); err != nil {
		return err
	}
	if r.Password == "" {
		return models.ErrFieldRequired("password")
	}
	return nil
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

func (r *ForgotPasswordRequest) Sanitize() {
	r.Email = strings.TrimSpace(r.Email)
}

func (r ForgotPasswordRequest) Validate() error {
	return models.ValidateEmail(r.Email)
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (r *ResetPasswordRequest) Sanitize() {
	r.Token = strings.TrimSpace(r.Token)
	r.NewPassword = strings.TrimSpace(r.NewPassword)
}

func (r ResetPasswordRequest) Validate() error {
	if r.Token == "" {
		return models.ErrFieldRequired("token")
	}
	if r.NewPassword == "" {
		return models.ErrFieldRequired("new_password")
	}
	return nil
}

type TransactionRequest struct {
	Ref        string `json:"ref"`
	Kind       string `json:"kind"`
	Points     int64  `json:"points"`
	OccurredAt string `json:"occurred_at"`
}

func (r *TransactionRequest) Sanitize() {
	r.Ref = strings.TrimSpace(r.Ref)
	r.Kind = strings.ToLower(strings.TrimSpace(r.Kind))
	r.OccurredAt = strings.TrimSpace(r.OccurredAt)
}

func (r TransactionRequest) Validate() error {
	if r.Ref == "" {
		return models.ErrFieldRequired("ref")
	}
	if r.Kind != models.KindEarn && r.Kind != models.KindSpend && r.Kind != models.KindAdjustment {
		return models.ErrInvalidKind
	}
	if r.Points <= 0 {
		return models.ErrInvalidPoints
	}
	if r.OccurredAt == "" {
		return models.ErrFieldRequired("occurred_at")
	}
	if _, err := time.Parse(time.RFC3339, r.OccurredAt); err != nil {
		return models.ErrFieldRequired("occurred_at")
	}
	return nil
}

func (r TransactionRequest) OccurredTime() (time.Time, error) {
	return time.Parse(time.RFC3339, r.OccurredAt)
}
