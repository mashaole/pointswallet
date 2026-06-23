package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"pointswallet/internal/models"
)

type LedgerDAO struct {
	db *sql.DB
}

func NewLedgerDAO(db *sql.DB) *LedgerDAO {
	return &LedgerDAO{db: db}
}

func (d *LedgerDAO) ListByAccount(ctx context.Context, accountID string, limit, offset int) ([]models.LedgerEntry, int, error) {
	var total int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ledger_entries WHERE account_id = $1`, accountID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ledger: %w", err)
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT id, ref, account_id, kind, points, balance_after_points,
		       occurred_at, recorded_at, actor_account_id, source
		FROM ledger_entries
		WHERE account_id = $1
		ORDER BY recorded_at DESC
		LIMIT $2 OFFSET $3`, accountID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list ledger: %w", err)
	}
	defer rows.Close()

	var entries []models.LedgerEntry
	for rows.Next() {
		var e models.LedgerEntry
		var pts, bal int64
		if err := rows.Scan(
			&e.ID, &e.Ref, &e.AccountID, &e.Kind, &pts, &bal,
			&e.OccurredAt, &e.RecordedAt, &e.ActorAccountID, &e.Source,
		); err != nil {
			return nil, 0, fmt.Errorf("scan ledger: %w", err)
		}
		e.Points = models.Points(pts)
		e.BalanceAfterPoints = models.Points(bal)
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}
