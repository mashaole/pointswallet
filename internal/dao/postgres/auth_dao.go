package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"pointswallet/internal/models"
)

type AuthDAO struct {
	db *sql.DB
}

func NewAuthDAO(db *sql.DB) *AuthDAO {
	return &AuthDAO{db: db}
}

func (d *AuthDAO) GetAccountByEmail(ctx context.Context, email string) (models.Account, error) {
	var acct models.Account
	var balance int64
	err := d.db.QueryRowContext(ctx, `
		SELECT account_id, name, email, password_hash, role, balance_points, created_at
		FROM accounts WHERE email = $1 AND deleted_at IS NULL`, email,
	).Scan(&acct.AccountID, &acct.Name, &acct.Email, &acct.PasswordHash, &acct.Role, &balance, &acct.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Account{}, models.ErrNotFound
	}
	if err != nil {
		return models.Account{}, fmt.Errorf("get by email: %w", err)
	}
	acct.BalancePoints = models.Points(balance)
	return acct, nil
}

func (d *AuthDAO) RevokeAllTokens(ctx context.Context, accountID string) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE auth_tokens SET revoked_at = now()
		WHERE account_id = $1 AND revoked_at IS NULL`, accountID,
	)
	return err
}

func (d *AuthDAO) CreateToken(ctx context.Context, id, accountID, token string, expiresAt time.Time) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO auth_tokens (id, account_id, token, expires_at)
		VALUES ($1, $2, $3, $4)`, id, accountID, token, expiresAt,
	)
	return err
}

func (d *AuthDAO) GetActiveToken(ctx context.Context, token string) (string, error) {
	var accountID string
	err := d.db.QueryRowContext(ctx, `
		SELECT account_id FROM auth_tokens
		WHERE token = $1 AND revoked_at IS NULL AND expires_at > now()`, token,
	).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", models.ErrUnauthorized
	}
	return accountID, err
}

func (d *AuthDAO) RevokeToken(ctx context.Context, token string) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE auth_tokens SET revoked_at = now()
		WHERE token = $1 AND revoked_at IS NULL`, token,
	)
	return err
}

func (d *AuthDAO) CreateResetToken(ctx context.Context, id, accountID, tokenHash string, expiresAt time.Time) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO password_reset_tokens (id, account_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)`, id, accountID, tokenHash, expiresAt,
	)
	return err
}

func (d *AuthDAO) UseResetToken(ctx context.Context, tokenHash string) (string, error) {
	var accountID string
	err := d.db.QueryRowContext(ctx, `
		UPDATE password_reset_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING account_id`, tokenHash,
	).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", models.ErrUnauthorized
	}
	return accountID, err
}

func (d *AuthDAO) UpdatePassword(ctx context.Context, accountID, passwordHash string) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE accounts SET password_hash = $1 WHERE account_id = $2 AND deleted_at IS NULL`, passwordHash, accountID,
	)
	return err
}

func (d *AuthDAO) SeedAdminIfMissing(ctx context.Context, acct models.Account) error {
	var exists bool
	err := d.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE account_id = $1)`, acct.AccountID,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = d.db.ExecContext(ctx, `
		INSERT INTO accounts (account_id, name, email, password_hash, role, balance_points)
		VALUES ($1, $2, $3, $4, $5, 0)
		ON CONFLICT DO NOTHING`,
		acct.AccountID, acct.Name, acct.Email, acct.PasswordHash, acct.Role,
	)
	return err
}

func (d *AuthDAO) IsAccountActive(ctx context.Context, accountID string) (bool, error) {
	var active bool
	err := d.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM accounts WHERE account_id = $1 AND deleted_at IS NULL
		)`, accountID,
	).Scan(&active)
	return active, err
}
