package jma

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidArgument は、Read の引数が条件を満たさないことを表す。
	ErrInvalidArgument = errors.New("jma: invalid argument")
	// ErrInvalidFormat は、ZIP 内の構成や CSV の形式が仕様と違うことを表す。
	ErrInvalidFormat = errors.New("jma: invalid format")
)

// ReadError は、入力の処理に失敗した場所と原因を持つ。
// Line は表題を含む1始まりの物理行番号で、行に結び付かないときは0。
type ReadError struct {
	Archive   string
	File      string
	Line      int
	StationID string
	Element   Element
	Field     string
	Err       error
}

// Error は、特定できた場所と原因を返す。
func (e *ReadError) Error() string {
	parts := []string{"jma", e.Archive}
	if e.File != "" {
		parts = append(parts, e.File)
	}
	if e.Line != 0 {
		parts = append(parts, fmt.Sprintf("line %d", e.Line))
	}
	if e.StationID != "" {
		parts = append(parts, "station "+e.StationID)
	}
	if e.Element != "" {
		parts = append(parts, "element "+string(e.Element))
	}
	if e.Field != "" {
		parts = append(parts, e.Field)
	}
	return strings.Join(parts, ": ") + ": " + e.Err.Error()
}

// Unwrap は、errors.Is と errors.As で調べられる元のエラーを返す。
func (e *ReadError) Unwrap() error { return e.Err }

func (e ReadError) fail(field string, err error) error {
	e.Field, e.Err = field, err
	return &e
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidFormat, fmt.Sprintf(format, args...))
}
