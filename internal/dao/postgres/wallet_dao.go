package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"pointswallet/internal/models"
)

type WalletDAO struct {
	db *sql.DB
}

func NewWalletDAO(db *sql.DB) *WalletDAO {
	return &WalletDAO{db: db}
}

func (d *WalletDAO) CreateAccount(ctx context.Context, acct models.Account) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO accounts (account_id, name, email, password_hash, role, balance_points)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		acct.AccountID, acct.Name, acct.Email, acct.PasswordHash, acct.Role, acct.BalancePoints.Int64(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			if strings.Contains(err.Error(), "accounts_email_unique") {
				return models.ErrEmailAlreadyExists
			}
		}
		if isCheckViolation(err) {
			return models.ErrInvalidEmail
		}
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

func (d *WalletDAO) GetAccount(ctx context.Context, accountID string) (models.Account, error) {
	var acct models.Account
	var balance int64
	err := d.db.QueryRowContext(ctx, `
		SELECT account_id, name, email, password_hash, role, balance_points, created_at
		FROM accounts WHERE account_id = $1`, accountID,
	).Scan(&acct.AccountID, &acct.Name, &acct.Email, &acct.PasswordHash, &acct.Role, &balance, &acct.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Account{}, models.ErrNotFound
	}
	if err != nil {
		return models.Account{}, fmt.Errorf("get account: %w", err)
	}
	acct.BalancePoints = models.Points(balance)
	return acct, nil
}

func (d *WalletDAO) GetBalance(ctx context.Context, accountID string) (models.Points, error) {
	var balance int64
	err := d.db.QueryRowContext(ctx,
		`SELECT balance_points FROM accounts WHERE account_id = $1`, accountID,
	).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, models.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get balance: %w", err)
	}
	return models.Points(balance), nil
}

func (d *WalletDAO) ApplyTransaction(ctx context.Context, in models.TransactionInput) (models.LedgerEntry, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return models.LedgerEntry{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var balance int64
	err = tx.QueryRowContext(ctx,
		`SELECT balance_points FROM accounts WHERE account_id = $1 FOR UPDATE`, in.AccountID,
	).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return models.LedgerEntry{}, models.ErrNotFound
	}
	if err != nil {
		return models.LedgerEntry{}, fmt.Errorf("lock account: %w", err)
	}

	var exists bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM ledger_entries WHERE ref = $1)`, in.Ref,
	).Scan(&exists)
	if err != nil {
		return models.LedgerEntry{}, fmt.Errorf("check ref: %w", err)
	}
	if exists {
		return models.LedgerEntry{}, models.ErrDuplicateRef
	}

	delta := models.PointsFromWhole(in.WholePoints)
	current := models.Points(balance)
	var newBalance models.Points

	switch in.Kind {
	case models.KindEarn, models.KindAdjustment:
		newBalance = current.Add(delta)
	case models.KindSpend:
		if current.Sub(delta) < 0 {
			return models.LedgerEntry{}, models.ErrInsufficientBalance
		}
		newBalance = current.Sub(delta)
	default:
		return models.LedgerEntry{}, models.ErrInvalidKind
	}

	var entry models.LedgerEntry
	var ptsStored, balStored int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO ledger_entries
			(ref, account_id, kind, points, balance_after_points, occurred_at, actor_account_id, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, ref, account_id, kind, points, balance_after_points, occurred_at, recorded_at, actor_account_id, source`,
		in.Ref, in.AccountID, in.Kind, delta.Int64(), newBalance.Int64(), in.OccurredAt, in.ActorAccountID, in.Source,
	).Scan(
		&entry.ID, &entry.Ref, &entry.AccountID, &entry.Kind,
		&ptsStored, &balStored, &entry.OccurredAt, &entry.RecordedAt,
		&entry.ActorAccountID, &entry.Source,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return models.LedgerEntry{}, models.ErrDuplicateRef
		}
		return models.LedgerEntry{}, fmt.Errorf("insert ledger: %w", err)
	}
	entry.Points = models.Points(ptsStored)
	entry.BalanceAfterPoints = models.Points(balStored)

	_, err = tx.ExecContext(ctx,
		`UPDATE accounts SET balance_points = $1 WHERE account_id = $2`,
		newBalance.Int64(), in.AccountID,
	)
	if err != nil {
		return models.LedgerEntry{}, fmt.Errorf("update balance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return models.LedgerEntry{}, fmt.Errorf("commit: %w", err)
	}
	return entry, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}

func isCheckViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23514")
}
