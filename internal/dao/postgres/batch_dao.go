package postgres

import (
	"context"
	"database/sql"
	"errors"

	"pointswallet/internal/dao"
	"pointswallet/internal/models"
)

type BatchDAO struct {
	db *sql.DB
}

func NewBatchDAO(db *sql.DB) *BatchDAO {
	return &BatchDAO{db: db}
}

func (d *BatchDAO) CreateJob(ctx context.Context, id string, totalRows int) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO batch_jobs (id, status, total_rows)
		VALUES ($1, $2, $3)`, id, models.BatchStatusQueued, totalRows,
	)
	return err
}

func (d *BatchDAO) UpdateJobStatus(ctx context.Context, id, status string) error {
	if status == models.BatchStatusProcessing {
		_, err := d.db.ExecContext(ctx, `
			UPDATE batch_jobs SET status = $1, started_at = now() WHERE id = $2`, status, id,
		)
		return err
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE batch_jobs SET status = $1 WHERE id = $2`, status, id,
	)
	return err
}

func (d *BatchDAO) CompleteJob(ctx context.Context, id string, summary models.BatchSummary) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE batch_jobs
		SET status = $1, accepted_count = $2, rejected_count = $3,
		    duplicate_count = $4, completed_at = now()
		WHERE id = $5`,
		models.BatchStatusCompleted, summary.Accepted, summary.Rejected, summary.Duplicates, id,
	)
	return err
}

func (d *BatchDAO) GetJob(ctx context.Context, id string) (dao.BatchJob, error) {
	var job dao.BatchJob
	var started, completed sql.NullTime
	var errMsg sql.NullString
	err := d.db.QueryRowContext(ctx, `
		SELECT id, status, total_rows, accepted_count, rejected_count, duplicate_count,
		       created_at, started_at, completed_at, error_message
		FROM batch_jobs WHERE id = $1`, id,
	).Scan(
		&job.ID, &job.Status, &job.TotalRows, &job.AcceptedCount, &job.RejectedCount,
		&job.DuplicateCount, &job.CreatedAt, &started, &completed, &errMsg,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return dao.BatchJob{}, models.ErrNotFound
	}
	if err != nil {
		return dao.BatchJob{}, err
	}
	if started.Valid {
		t := started.Time
		job.StartedAt = &t
	}
	if completed.Valid {
		t := completed.Time
		job.CompletedAt = &t
	}
	if errMsg.Valid {
		s := errMsg.String
		job.ErrorMessage = &s
	}
	return job, nil
}

func (d *BatchDAO) RecoverStaleJobs(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE batch_jobs
		SET status = $1, error_message = 'interrupted during processing', completed_at = now()
		WHERE status = $2`,
		models.BatchStatusFailed, models.BatchStatusProcessing,
	)
	return err
}
