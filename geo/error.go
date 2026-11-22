package geo

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidArgument = errors.New("geo: invalid argument")
	ErrInvalidFormat   = errors.New("geo: invalid format")
)

// ReadError は入力の処理に失敗した場所と原因を持つ。
type ReadError struct {
	Source string
	File   string
	Line   int
	Key    string
	Field  string
	Err    error
}

func (e *ReadError) Error() string {
	parts := []string{"geo"}
	for _, s := range []string{e.Source, e.File} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if e.Line != 0 {
		parts = append(parts, fmt.Sprintf("line %d", e.Line))
	}
	for _, s := range []string{e.Key, e.Field} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ": ") + ": " + e.Err.Error()
}

func (e *ReadError) Unwrap() error { return e.Err }

func (e ReadError) fail(field string, err error) error {
	e.Field, e.Err = field, err
	return &e
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidFormat, fmt.Sprintf(format, args...))
}
