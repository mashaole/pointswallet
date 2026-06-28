package models

import (
	"errors"
	"fmt"
)

var (
	ErrValidation          = errors.New("validation error")
	ErrInvalidEmail        = fmt.Errorf("%w: invalid email", ErrValidation)
	ErrInvalidRole         = fmt.Errorf("%w: invalid role", ErrValidation)
	ErrInvalidKind         = fmt.Errorf("%w: invalid kind", ErrValidation)
	ErrInvalidDirection    = fmt.Errorf("%w: direction must be credit or debit", ErrValidation)
	ErrInvalidPoints       = fmt.Errorf("%w: points must be positive", ErrValidation)
	ErrForbidden           = errors.New("forbidden")
	ErrDuplicateRef        = errors.New("duplicate ref")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrNotFound            = errors.New("not found")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrEmailAlreadyExists  = errors.New("email already exists")
	ErrAccountAlreadyExists = errors.New("account already exists")
	ErrLastAdmin            = errors.New("last admin")
)

type APIError struct {
	Code    string
	Message string
	Status  int
}

func (e APIError) Error() string {
	return e.Message
}

func NewAPIError(code, message string, status int) APIError {
	return APIError{Code: code, Message: message, Status: status}
}
