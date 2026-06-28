package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"pointswallet/internal/models"
)

const activeAccountSQL = `deleted_at IS NULL`

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
		return mapAccountInsertErr(err)
	}
	return nil
}

func (d *WalletDAO) GetAccount(ctx context.Context, accountID string) (models.Account, error) {
	return d.scanAccount(d.db.QueryRowContext(ctx, `
		SELECT account_id, name, email, password_hash, role, balance_points, created_at
		FROM accounts WHERE account_id = $1 AND `+activeAccountSQL, accountID,
	))
}

func (d *WalletDAO) ListAccounts(ctx context.Context, limit, offset int) ([]models.Account, int, error) {
	var total int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE `+activeAccountSQL,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count accounts: %w", err)
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT account_id, name, email, role, balance_points, created_at
		FROM accounts
		WHERE `+activeAccountSQL+`
		ORDER BY created_at ASC, account_id ASC
		LIMIT $1 OFFSET $2`, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]models.Account, 0, limit)
	for rows.Next() {
		var acct models.Account
		var balance int64
		if err := rows.Scan(&acct.AccountID, &acct.Name, &acct.Email, &acct.Role, &balance, &acct.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan account: %w", err)
		}
		acct.BalancePoints = models.Points(balance)
		accounts = append(accounts, acct)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list accounts rows: %w", err)
	}
	return accounts, total, nil
}

func (d *WalletDAO) UpdateProfile(ctx context.Context, accountID, name, email string) (models.Account, error) {
	return d.scanAccount(d.db.QueryRowContext(ctx, `
		UPDATE accounts SET name = $2, email = $3
		WHERE account_id = $1 AND `+activeAccountSQL+`
		RETURNING account_id, name, email, password_hash, role, balance_points, created_at`,
		accountID, name, email,
	))
}

func (d *WalletDAO) UpdateProfileRole(ctx context.Context, accountID, name, email, role string) (models.Account, error) {
	return d.scanAccount(d.db.QueryRowContext(ctx, `
		UPDATE accounts SET name = $2, email = $3, role = $4
		WHERE account_id = $1 AND `+activeAccountSQL+`
		RETURNING account_id, name, email, password_hash, role, balance_points, created_at`,
		accountID, name, email, role,
	))
}

func (d *WalletDAO) SoftDeleteAccount(ctx context.Context, accountID, anonymizedEmail string) error {
	res, err := d.db.ExecContext(ctx, `
		UPDATE accounts SET deleted_at = now(), email = $2
		WHERE account_id = $1 AND `+activeAccountSQL,
		accountID, anonymizedEmail,
	)
	if err != nil {
		return mapAccountInsertErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("soft delete rows affected: %w", err)
	}
	if n == 0 {
		return models.ErrNotFound
	}
	return nil
}

func (d *WalletDAO) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM accounts WHERE role = $1 AND `+activeAccountSQL,
		models.RoleAdmin,
	).Scan(&n)
	return n, err
}

func (d *WalletDAO) scanAccount(row *sql.Row) (models.Account, error) {
	var acct models.Account
	var balance int64
	err := row.Scan(&acct.AccountID, &acct.Name, &acct.Email, &acct.PasswordHash, &acct.Role, &balance, &acct.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Account{}, models.ErrNotFound
	}
	if err != nil {
		return models.Account{}, mapAccountInsertErr(err)
	}
	acct.BalancePoints = models.Points(balance)
	return acct, nil
}

func (d *WalletDAO) GetBalance(ctx context.Context, accountID string) (models.Points, error) {
	var balance int64
	err := d.db.QueryRowContext(ctx,
		`SELECT balance_points FROM accounts WHERE account_id = $1 AND `+activeAccountSQL, accountID,
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
		`SELECT balance_points FROM accounts WHERE account_id = $1 AND `+activeAccountSQL+` FOR UPDATE`, in.AccountID,
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
	direction, err := models.ResolveTransactionDirection(in.Kind, in.Direction)
	if err != nil {
		return models.LedgerEntry{}, err
	}
	var newBalance models.Points

	switch in.Kind {
	case models.KindEarn:
		newBalance = current.Add(delta)
	case models.KindAdjustment:
		if direction == models.DirectionDebit {
			if current.Sub(delta) < 0 {
				return models.LedgerEntry{}, models.ErrInsufficientBalance
			}
			newBalance = current.Sub(delta)
		} else {
			newBalance = current.Add(delta)
		}
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
			(ref, account_id, kind, direction, points, balance_after_points, occurred_at, actor_account_id, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, ref, account_id, kind, direction, points, balance_after_points, occurred_at, recorded_at, actor_account_id, source`,
		in.Ref, in.AccountID, in.Kind, direction, delta.Int64(), newBalance.Int64(), in.OccurredAt, in.ActorAccountID, in.Source,
	).Scan(
		&entry.ID, &entry.Ref, &entry.AccountID, &entry.Kind, &entry.Direction,
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
		`UPDATE accounts SET balance_points = $1 WHERE account_id = $2 AND `+activeAccountSQL,
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
