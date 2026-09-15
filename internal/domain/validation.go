package domain

import (
	"errors"
	"sort"
	"strings"
)

// Error tambahan untuk lapisan service/transport.
var (
	ErrValidation    = errors.New("permintaan tidak valid")
	ErrInvalidCursor = errors.New("cursor tidak valid")
	ErrInvalidToken  = errors.New("token tidak valid atau kedaluwarsa")
)

// ValidationError membawa detail per field. errors.Is(err, ErrValidation) benar untuknya.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+e.Fields[k])
	}
	return "permintaan tidak valid: " + strings.Join(parts, "; ")
}

// Is membuat errors.Is(err, ErrValidation) bernilai benar.
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

// NewValidationError membuat error validasi; nil bila tidak ada field bermasalah.
func NewValidationError(fields map[string]string) error {
	if len(fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: fields}
}
