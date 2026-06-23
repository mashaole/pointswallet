package wallet

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"pointswallet/internal/dao"
	"pointswallet/internal/models"
)

type Service struct {
	wallet dao.WalletDAO
	ledger dao.LedgerDAO
}

func NewService(wallet dao.WalletDAO, ledger dao.LedgerDAO) *Service {
	return &Service{wallet: wallet, ledger: ledger}
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
