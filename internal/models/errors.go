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
	ErrInvalidPoints       = fmt.Errorf("%w: points must be positive", ErrValidation)
	ErrDuplicateRef        = errors.New("duplicate ref")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrNotFound            = errors.New("not found")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrEmailAlreadyExists  = errors.New("email already exists")
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
