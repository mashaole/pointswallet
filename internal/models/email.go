package models

import (
	"fmt"
	"net/mail"
	"strings"
)

func NormalizeEmail(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", ErrFieldRequired("email")
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", ErrInvalidEmail
	}
	return strings.ToLower(addr.Address), nil
}

func ValidateEmail(raw string) error {
	_, err := NormalizeEmail(raw)
	return err
}

func ErrFieldRequired(field string) error {
	return fmt.Errorf("%w: %s", ErrValidation, field)
}
