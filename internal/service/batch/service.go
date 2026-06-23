package batch

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"pointswallet/internal/dao"
	"pointswallet/internal/models"
	walletsvc "pointswallet/internal/service/wallet"
)

type Service struct {
	batch  dao.BatchDAO
	audit  dao.AuditDAO
	wallet *walletsvc.Service
	workers int
}

func NewService(batch dao.BatchDAO, audit dao.AuditDAO, wallet *walletsvc.Service, workers int) *Service {
	if workers < 1 {
		workers = 1
	}
	return &Service{batch: batch, audit: audit, wallet: wallet, workers: workers}
}

func (s *Service) RecoverStaleJobs(ctx context.Context) error {
	return s.batch.RecoverStaleJobs(ctx)
}

func (s *Service) CreateJob(ctx context.Context, jobID string, rows []models.BatchRow) error {
	return s.batch.CreateJob(ctx, jobID, len(rows))
}

func (s *Service) GetJob(ctx context.Context, id string) (dao.BatchJob, error) {
	return s.batch.GetJob(ctx, id)
}

func (s *Service) ListAudit(ctx context.Context, batchID string, limit, offset int) ([]dao.AuditEvent, int, error) {
	return s.audit.ListByBatchID(ctx, batchID, limit, offset)
}

func (s *Service) ProcessAsync(ctx context.Context, jobID string, rows []models.BatchRow, actorAccountID string) {
	go func() {
		bgCtx := context.Background()
		_ = s.batch.UpdateJobStatus(bgCtx, jobID, models.BatchStatusProcessing)

		rowCh := make(chan models.BatchRow, len(rows))
		var wg sync.WaitGroup
		var mu sync.Mutex
		summary := models.BatchSummary{}

		for i := 0; i < s.workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for row := range rowCh {
					err := s.processRow(bgCtx, jobID, row, actorAccountID)
					mu.Lock()
					summary.Record(err)
					mu.Unlock()
				}
			}()
		}
		for _, r := range rows {
			rowCh <- r
		}
		close(rowCh)
		wg.Wait()
		_ = s.batch.CompleteJob(bgCtx, jobID, summary)
	}()
}

func (s *Service) processRow(ctx context.Context, jobID string, row models.BatchRow, actorAccountID string) error {
	in := models.TransactionInput{
		Ref:            row.Ref,
		AccountID:      row.AccountID,
		Kind:           row.Kind,
		WholePoints:    row.WholePoints,
		OccurredAt:     row.OccurredAt,
		ActorAccountID: actorAccountID,
		Source:         models.SourceBatch,
	}
	_, err := s.wallet.ApplyTransaction(ctx, in)
	status := models.AuditAccepted
	reason := "ok"
	if err != nil {
		status = models.AuditRejected
		reason = auditReason(err)
	}
	_ = s.audit.InsertAuditEvent(ctx, jobID, row.Ref, row.AccountID, status, reason)
	return err
}

func auditReason(err error) string {
	switch {
	case errors.Is(err, models.ErrDuplicateRef):
		return "duplicate_ref"
	case errors.Is(err, models.ErrInsufficientBalance):
		return "insufficient_balance"
	case errors.Is(err, models.ErrNotFound):
		return "account_not_found"
	case errors.Is(err, models.ErrInvalidKind), errors.Is(err, models.ErrInvalidPoints):
		return "validation_error"
	default:
		return "internal_error"
	}
}

func ParseCSV(r io.Reader) ([]models.BatchRow, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}
	if len(header) < 5 {
		return nil, fmt.Errorf("%w: csv must have ref,account_id,kind,points,occurred_at", models.ErrValidation)
	}
	expected := []string{"ref", "account_id", "kind", "points", "occurred_at"}
	for i, col := range expected {
		if !strings.EqualFold(strings.TrimSpace(header[i]), col) {
			return nil, fmt.Errorf("%w: invalid csv header", models.ErrValidation)
		}
	}

	var rows []models.BatchRow
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv row: %w", err)
		}
		if len(record) < 5 {
			continue
		}
		var wholePoints int64
		if _, err := fmt.Sscan(record[3], &wholePoints); err != nil || wholePoints <= 0 {
			return nil, fmt.Errorf("%w: invalid points in csv", models.ErrValidation)
		}
		occurredAt, err := time.Parse(time.RFC3339, strings.TrimSpace(record[4]))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid occurred_at in csv", models.ErrValidation)
		}
		rows = append(rows, models.BatchRow{
			Ref:         strings.TrimSpace(record[0]),
			AccountID:   strings.TrimSpace(record[1]),
			Kind:        strings.ToLower(strings.TrimSpace(record[2])),
			WholePoints: wholePoints,
			OccurredAt:  occurredAt,
		})
	}
	return rows, nil
}

func NewJobID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
