package wallet_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"pointswallet/internal/models"
	"pointswallet/internal/service/wallet"
)

type mockWalletDAO struct {
	balance models.Points
	refs    map[string]bool
}

func (m *mockWalletDAO) CreateAccount(ctx context.Context, acct models.Account) error {
	return nil
}
func (m *mockWalletDAO) GetAccount(ctx context.Context, accountID string) (models.Account, error) {
	return models.Account{}, nil
}
func (m *mockWalletDAO) ListAccounts(ctx context.Context, limit, offset int) ([]models.Account, int, error) {
	return nil, 0, nil
}
func (m *mockWalletDAO) UpdateProfile(ctx context.Context, accountID, name, email string) (models.Account, error) {
	return models.Account{}, nil
}
func (m *mockWalletDAO) UpdateProfileRole(ctx context.Context, accountID, name, email, role string) (models.Account, error) {
	return models.Account{}, nil
}
func (m *mockWalletDAO) SoftDeleteAccount(ctx context.Context, accountID, anonymizedEmail string) error {
	return nil
}
func (m *mockWalletDAO) CountActiveAdmins(ctx context.Context) (int, error) {
	return 1, nil
}
func (m *mockWalletDAO) GetBalance(ctx context.Context, accountID string) (models.Points, error) {
	return m.balance, nil
}
func (m *mockWalletDAO) ApplyTransaction(ctx context.Context, in models.TransactionInput) (models.LedgerEntry, error) {
	if in.WholePoints <= 0 {
		return models.LedgerEntry{}, models.ErrInvalidPoints
	}
	if m.refs[in.Ref] {
		return models.LedgerEntry{}, models.ErrDuplicateRef
	}
	delta := models.PointsFromWhole(in.WholePoints)
	direction, err := models.ResolveTransactionDirection(in.Kind, in.Direction)
	if err != nil {
		return models.LedgerEntry{}, err
	}
	switch in.Kind {
	case models.KindEarn:
		m.balance = m.balance.Add(delta)
	case models.KindAdjustment:
		if direction == models.DirectionDebit {
			if m.balance.Sub(delta) < 0 {
				return models.LedgerEntry{}, models.ErrInsufficientBalance
			}
			m.balance = m.balance.Sub(delta)
		} else {
			m.balance = m.balance.Add(delta)
		}
	case models.KindSpend:
		if m.balance.Sub(delta) < 0 {
			return models.LedgerEntry{}, models.ErrInsufficientBalance
		}
		m.balance = m.balance.Sub(delta)
	default:
		return models.LedgerEntry{}, models.ErrInvalidKind
	}
	m.refs[in.Ref] = true
	return models.LedgerEntry{
		Ref: in.Ref, Kind: in.Kind, Direction: direction,
		Points: delta, BalanceAfterPoints: m.balance,
	}, nil
}

type mockLedgerDAO struct{}

func (m *mockLedgerDAO) ListByAccount(ctx context.Context, accountID string, limit, offset int) ([]models.LedgerEntry, int, error) {
	return nil, 0, nil
}

func TestWalletService_Spend_InsufficientBalance(t *testing.T) {
	dao := &mockWalletDAO{balance: models.PointsFromWhole(10), refs: map[string]bool{}}
	svc := wallet.NewService(dao, &mockLedgerDAO{}, nil)
	_, err := svc.ApplyTransaction(context.Background(), models.TransactionInput{
		Ref: "tx-1", AccountID: "a1", Kind: models.KindSpend, Direction: models.DirectionDebit, WholePoints: 50,
		OccurredAt: time.Now(), ActorAccountID: "a1", Source: models.SourceAPI,
	})
	if !errors.Is(err, models.ErrInsufficientBalance) {
		t.Fatalf("expected insufficient balance, got %v", err)
	}
}

func TestWalletService_AdjustmentDebit_InsufficientBalance(t *testing.T) {
	dao := &mockWalletDAO{balance: models.PointsFromWhole(5), refs: map[string]bool{}}
	svc := wallet.NewService(dao, &mockLedgerDAO{}, nil)
	_, err := svc.ApplyTransaction(context.Background(), models.TransactionInput{
		Ref: "adj-1", AccountID: "a1", Kind: models.KindAdjustment,
		Direction: models.DirectionDebit, WholePoints: 10,
		OccurredAt: time.Now(), ActorAccountID: "admin", Source: models.SourceAPI,
	})
	if !errors.Is(err, models.ErrInsufficientBalance) {
		t.Fatalf("expected insufficient balance, got %v", err)
	}
}

func TestWalletService_DuplicateRef(t *testing.T) {
	dao := &mockWalletDAO{balance: models.PointsFromWhole(100), refs: map[string]bool{}}
	svc := wallet.NewService(dao, &mockLedgerDAO{}, nil)
	in := models.TransactionInput{
		Ref: "tx-dup", AccountID: "a1", Kind: models.KindEarn, Direction: models.DirectionCredit, WholePoints: 10,
		OccurredAt: time.Now(), ActorAccountID: "a1", Source: models.SourceAPI,
	}
	if _, err := svc.ApplyTransaction(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ApplyTransaction(context.Background(), in)
	if !errors.Is(err, models.ErrDuplicateRef) {
		t.Fatalf("expected duplicate ref, got %v", err)
	}
}
