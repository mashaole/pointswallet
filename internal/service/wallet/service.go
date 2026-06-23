package wallet

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"pointswallet/internal/dao"
	"pointswallet/internal/models"
)

type sessionRevoker interface {
	RevokeAllTokens(ctx context.Context, accountID string) error
}

type Service struct {
	wallet   dao.WalletDAO
	ledger   dao.LedgerDAO
	sessions sessionRevoker
}

func NewService(wallet dao.WalletDAO, ledger dao.LedgerDAO, sessions sessionRevoker) *Service {
	return &Service{wallet: wallet, ledger: ledger, sessions: sessions}
}

type CreateAccountInput struct {
	AccountID string
	Name      string
	Email     string
	Password  string
	Role      string
}

func (s *Service) CreateAccount(ctx context.Context, in CreateAccountInput) (models.Account, error) {
	email, err := models.NormalizeEmail(in.Email)
	if err != nil {
		return models.Account{}, err
	}
	if in.Role != models.RoleMember && in.Role != models.RoleAdmin {
		return models.Account{}, models.ErrInvalidRole
	}
	if len(in.Password) < 8 {
		return models.Account{}, fmt.Errorf("%w: password must be at least 8 characters", models.ErrValidation)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return models.Account{}, fmt.Errorf("hash password: %w", err)
	}
	acct := models.Account{
		AccountID:     in.AccountID,
		Name:          in.Name,
		Email:         email,
		PasswordHash:  string(hash),
		Role:          in.Role,
		BalancePoints: 0,
	}
	if err := s.wallet.CreateAccount(ctx, acct); err != nil {
		return models.Account{}, err
	}
	return acct, nil
}

func (s *Service) GetAccount(ctx context.Context, accountID string) (models.Account, error) {
	return s.wallet.GetAccount(ctx, accountID)
}

func (s *Service) ListAccounts(ctx context.Context, limit, offset int) ([]models.Account, int, error) {
	return s.wallet.ListAccounts(ctx, limit, offset)
}

func (s *Service) UpdateMemberProfile(ctx context.Context, accountID, name, email string) (models.Account, error) {
	normalized, err := models.NormalizeEmail(email)
	if err != nil {
		return models.Account{}, err
	}
	return s.wallet.UpdateProfile(ctx, accountID, name, normalized)
}

func (s *Service) UpdateAccountAsAdmin(ctx context.Context, accountID, name, email, role string) (models.Account, error) {
	normalized, err := models.NormalizeEmail(email)
	if err != nil {
		return models.Account{}, err
	}
	if role != models.RoleMember && role != models.RoleAdmin {
		return models.Account{}, models.ErrInvalidRole
	}
	current, err := s.wallet.GetAccount(ctx, accountID)
	if err != nil {
		return models.Account{}, err
	}
	if current.Role == models.RoleAdmin && role == models.RoleMember {
		if err := s.ensureNotLastAdmin(ctx); err != nil {
			return models.Account{}, err
		}
	}
	return s.wallet.UpdateProfileRole(ctx, accountID, name, normalized, role)
}

func (s *Service) SoftDeleteAccount(ctx context.Context, accountID string) error {
	acct, err := s.wallet.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if acct.Role == models.RoleAdmin {
		if err := s.ensureNotLastAdmin(ctx); err != nil {
			return err
		}
	}
	anonEmail := fmt.Sprintf("deleted+%s+%d@deleted.invalid", accountID, time.Now().UnixNano())
	if err := s.wallet.SoftDeleteAccount(ctx, accountID, anonEmail); err != nil {
		return err
	}
	if s.sessions != nil {
		_ = s.sessions.RevokeAllTokens(ctx, accountID)
	}
	return nil
}

func (s *Service) ensureNotLastAdmin(ctx context.Context) error {
	n, err := s.wallet.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return models.ErrLastAdmin
	}
	return nil
}

func (s *Service) GetBalance(ctx context.Context, accountID string) (models.Points, error) {
	return s.wallet.GetBalance(ctx, accountID)
}

func (s *Service) ApplyTransaction(ctx context.Context, in models.TransactionInput) (models.LedgerEntry, error) {
	if in.WholePoints <= 0 {
		return models.LedgerEntry{}, models.ErrInvalidPoints
	}
	switch in.Kind {
	case models.KindEarn, models.KindSpend, models.KindAdjustment:
	default:
		return models.LedgerEntry{}, models.ErrInvalidKind
	}
	return s.wallet.ApplyTransaction(ctx, in)
}

func (s *Service) ListLedger(ctx context.Context, accountID string, limit, offset int) ([]models.LedgerEntry, int, error) {
	return s.ledger.ListByAccount(ctx, accountID, limit, offset)
}
