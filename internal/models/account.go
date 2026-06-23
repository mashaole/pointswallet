package models

import (
	"errors"
	"time"
)

type Account struct {
	AccountID     string
	Name          string
	Email         string
	PasswordHash  string
	Role          string
	BalancePoints Points
	CreatedAt     time.Time
}

type LedgerEntry struct {
	ID                 int64
	Ref                string
	AccountID          string
	Kind               string
	Points             Points
	BalanceAfterPoints Points
	OccurredAt         time.Time
	RecordedAt         time.Time
	ActorAccountID     string
	Source             string
}

type TransactionInput struct {
	Ref            string
	AccountID      string
	Kind           string
	WholePoints    int64
	OccurredAt     time.Time
	ActorAccountID string
	Source         string
}

type BatchRow struct {
	Ref        string
	AccountID  string
	Kind       string
	WholePoints int64
	OccurredAt time.Time
}

type BatchSummary struct {
	Accepted   int
	Rejected   int
	Duplicates int
}

func (s *BatchSummary) Record(err error) {
	if err == nil {
		s.Accepted++
		return
	}
	if errors.Is(err, ErrDuplicateRef) {
		s.Duplicates++
		return
	}
	s.Rejected++
}
