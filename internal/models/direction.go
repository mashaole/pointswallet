package models

import (
	"strings"
)

// ResolveTransactionDirection validates required direction and maps kind to ledger direction.
// earn → credit, spend → debit, adjustment → credit | debit.
func ResolveTransactionDirection(kind, dir string) (string, error) {
	dir = strings.ToLower(strings.TrimSpace(dir))
	if dir == "" {
		return "", ErrFieldRequired("direction")
	}
	switch kind {
	case KindEarn:
		if dir != DirectionCredit {
			return "", ErrInvalidDirection
		}
		return DirectionCredit, nil
	case KindSpend:
		if dir != DirectionDebit {
			return "", ErrInvalidDirection
		}
		return DirectionDebit, nil
	case KindAdjustment:
		if dir != DirectionCredit && dir != DirectionDebit {
			return "", ErrInvalidDirection
		}
		return dir, nil
	default:
		return "", ErrInvalidKind
	}
}

// LedgerEntryDirection returns the audit direction for a stored ledger row.
func LedgerEntryDirection(kind, storedDirection string) string {
	if storedDirection == DirectionCredit || storedDirection == DirectionDebit {
		return storedDirection
	}
	if kind == KindSpend {
		return DirectionDebit
	}
	return DirectionCredit
}
