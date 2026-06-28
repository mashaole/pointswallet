package models_test

import (
	"errors"
	"testing"

	"pointswallet/internal/models"
)

func TestResolveTransactionDirection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    string
		dir     string
		want    string
		wantErr error
	}{
		{name: "earn requires credit", kind: models.KindEarn, dir: "credit", want: models.DirectionCredit},
		{name: "spend requires debit", kind: models.KindSpend, dir: "debit", want: models.DirectionDebit},
		{name: "adjustment credit", kind: models.KindAdjustment, dir: "credit", want: models.DirectionCredit},
		{name: "adjustment debit", kind: models.KindAdjustment, dir: "debit", want: models.DirectionDebit},
		{name: "missing direction", kind: models.KindEarn, wantErr: models.ErrValidation},
		{name: "earn rejects debit", kind: models.KindEarn, dir: "debit", wantErr: models.ErrInvalidDirection},
		{name: "spend rejects credit", kind: models.KindSpend, dir: "credit", wantErr: models.ErrInvalidDirection},
		{name: "invalid adjustment direction", kind: models.KindAdjustment, dir: "sideways", wantErr: models.ErrInvalidDirection},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := models.ResolveTransactionDirection(tt.kind, tt.dir)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestLedgerEntryDirection(t *testing.T) {
	t.Parallel()
	if got := models.LedgerEntryDirection(models.KindSpend, ""); got != models.DirectionDebit {
		t.Fatalf("spend fallback got %q", got)
	}
	if got := models.LedgerEntryDirection(models.KindAdjustment, models.DirectionDebit); got != models.DirectionDebit {
		t.Fatalf("stored debit got %q", got)
	}
}
