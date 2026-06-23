package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
	"golang.org/x/crypto/bcrypt"

	"pointswallet/internal/dao"
	"pointswallet/internal/models"
)

type Service struct {
	auth      dao.AuthDAO
	jwtSecret string
	jwtTTL    time.Duration
}

type LoginResult struct {
	AccessToken string
	AccountID   string
	Role        string
}

type ForgotPasswordResult struct {
	Message   string
	ResetToken string // dev stub: returned in response
}

func NewService(auth dao.AuthDAO, jwtSecret string, jwtTTL time.Duration) *Service {
	return &Service{auth: auth, jwtSecret: jwtSecret, jwtTTL: jwtTTL}
}

func (s *Service) Login(ctx context.Context, emailRaw, password string) (LoginResult, error) {
	email, err := models.NormalizeEmail(emailRaw)
	if err != nil {
		return LoginResult{}, models.ErrUnauthorized
	}
	acct, err := s.auth.GetAccountByEmail(ctx, email)
	if err != nil {
		return LoginResult{}, models.ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(acct.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, models.ErrUnauthorized
	}
	if err := s.auth.RevokeAllSessions(ctx, acct.AccountID); err != nil {
		return LoginResult{}, fmt.Errorf("revoke sessions: %w", err)
	}
	jti := newJTI()
	sessionID := newJTI()
	expiresAt := time.Now().Add(s.jwtTTL)
	if err := s.auth.CreateSession(ctx, sessionID, acct.AccountID, jti, expiresAt); err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}
	token, err := SignToken(s.jwtSecret, NewClaims(acct.AccountID, acct.Role, jti, s.jwtTTL))
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: token, AccountID: acct.AccountID, Role: acct.Role}, nil
}

func (s *Service) Logout(ctx context.Context, jti string) error {
	return s.auth.RevokeSession(ctx, jti)
}

func (s *Service) ValidateSession(ctx context.Context, claims Claims) error {
	accountID, err := s.auth.GetActiveSession(ctx, claims.JTI)
	if err != nil {
		return models.ErrUnauthorized
	}
	if accountID != claims.Sub {
		return models.ErrUnauthorized
	}
	return nil
}

func (s *Service) ForgotPassword(ctx context.Context, emailRaw string) (ForgotPasswordResult, error) {
	email, err := models.NormalizeEmail(emailRaw)
	if err != nil {
		return ForgotPasswordResult{Message: "If the email exists, a reset link was sent."}, nil
	}
	acct, err := s.auth.GetAccountByEmail(ctx, email)
	if err != nil {
		return ForgotPasswordResult{Message: "If the email exists, a reset link was sent."}, nil
	}
	_ = s.auth.RevokeAllSessions(ctx, acct.AccountID)
	token := newJTI()
	hash := hashToken(token)
	expires := time.Now().Add(time.Hour)
	_ = s.auth.CreateResetToken(ctx, newJTI(), acct.AccountID, hash, expires)
	return ForgotPasswordResult{
		Message:    "If the email exists, a reset link was sent.",
		ResetToken: token,
	}, nil
}

func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", models.ErrValidation)
	}
	hash := hashToken(token)
	accountID, err := s.auth.UseResetToken(ctx, hash)
	if err != nil {
		return models.ErrUnauthorized
	}
	pwHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.auth.UpdatePassword(ctx, accountID, string(pwHash)); err != nil {
		return err
	}
	return s.auth.RevokeAllSessions(ctx, accountID)
}

func (s *Service) SeedAdmin(ctx context.Context, accountID, name, email, password string) error {
	normalized, err := models.NormalizeEmail(email)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.auth.SeedAdminIfMissing(ctx, models.Account{
		AccountID:    accountID,
		Name:         name,
		Email:        normalized,
		PasswordHash: string(hash),
		Role:         models.RoleAdmin,
	})
}

func newJTI() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
