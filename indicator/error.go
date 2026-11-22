package indicator

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidDefinition は、定義が条件を満たさないことを表す。
	ErrInvalidDefinition = errors.New("indicator: invalid definition")
	// ErrInvalidArgument は、対象地域が条件を満たさないことを表す。
	ErrInvalidArgument = errors.New("indicator: invalid argument")
	// ErrInvalidRecord は、入力行が条件を満たさないことを表す。
	ErrInvalidRecord = errors.New("indicator: invalid record")
	// ErrDuplicateRecord は、同じ項目・地域・年の行が重複することを表す。
	ErrDuplicateRecord = errors.New("indicator: duplicate record")
)

// DefinitionError は、定義の誤ったフィールドと原因を持つ。
type DefinitionError struct {
	Field string
	Err   error
}

// Error は、定義の誤った場所と原因を返す。
func (e *DefinitionError) Error() string {
	return fmt.Sprintf("indicator: %s: %v", e.Field, e.Err)
}

// Unwrap は、分類用のエラーを含む原因を返す。
func (e *DefinitionError) Unwrap() error { return e.Err }

// InputError は、元の引数の位置、行の識別情報、誤ったフィールドと原因を持つ。
// Index は0始まりで、位置が無い場合だけ -1。
type InputError struct {
	Index int
	Area  string
	Item  Item
	Year  int
	Field string
	Err   error
}

// Error は、入力の誤った場所と原因を返す。
func (e *InputError) Error() string {
	return fmt.Sprintf("indicator: %s (index %d, area %q, item %q/%q, year %d): %v",
		e.Field, e.Index, e.Area, e.Item.TableID, e.Item.Code, e.Year, e.Err)
}

// Unwrap は、分類用のエラーを含む原因を返す。
func (e *InputError) Unwrap() error { return e.Err }
