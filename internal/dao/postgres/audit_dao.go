package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"pointswallet/internal/dao"
)

type AuditDAO struct {
	db *sql.DB
}

func NewAuditDAO(db *sql.DB) *AuditDAO {
	return &AuditDAO{db: db}
}

func (d *AuditDAO) InsertAuditEvent(ctx context.Context, batchID, ref, accountID, status, reason string) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO audit_events (batch_id, ref, account_id, status, reason)
		VALUES ($1, $2, $3, $4, $5)`, batchID, ref, accountID, status, reason,
	)
	return err
}

func (d *AuditDAO) ListByBatchID(ctx context.Context, batchID string, limit, offset int) ([]dao.AuditEvent, int, error) {
	var total int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE batch_id = $1`, batchID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit: %w", err)
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT id, batch_id, ref, account_id, status, reason, created_at
		FROM audit_events
		WHERE batch_id = $1
		ORDER BY id ASC
		LIMIT $2 OFFSET $3`, batchID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()

	var events []dao.AuditEvent
	for rows.Next() {
		var e dao.AuditEvent
		var ref, accountID sql.NullString
		if err := rows.Scan(&e.ID, &e.BatchID, &ref, &accountID, &e.Status, &e.Reason, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		if ref.Valid {
			e.Ref = ref.String
		}
		if accountID.Valid {
			e.AccountID = accountID.String
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}
