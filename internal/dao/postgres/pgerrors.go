package postgres

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"pointswallet/internal/models"
)

func mapAccountInsertErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			switch pgErr.ConstraintName {
			case "accounts_email_unique", "accounts_email_active_unique":
				return models.ErrEmailAlreadyExists
			case "accounts_pkey":
				return models.ErrAccountAlreadyExists
			default:
				return models.ErrAccountAlreadyExists
			}
		case "23514":
			return models.ErrInvalidEmail
		}
	}
	if err != nil && strings.Contains(err.Error(), "23505") {
		msg := err.Error()
		if strings.Contains(msg, "accounts_email_unique") || strings.Contains(msg, "accounts_email_active_unique") {
			return models.ErrEmailAlreadyExists
		}
		return models.ErrAccountAlreadyExists
	}
	if err != nil && strings.Contains(err.Error(), "23514") {
		return models.ErrInvalidEmail
	}
	return err
}
