// Package indicator は、e-Stat の行から定義に従って指標を計算する。
package indicator

import (
	"fmt"
	"math/big"
	"strings"
)

// Operation は、指標に使う計算を表す。
type Operation int

// OperationUnset 以降は、未設定、単一の値、比率、期間内の差の合計を区別する。
const (
	OperationUnset Operation = iota
	OperationValue
	OperationRatio
	OperationDiff
)

// YearRule は、基準年の決め方を表す。
type YearRule int

// YearRuleUnset 以降は、未設定、最新の共通年、指定年を区別する。
const (
	YearRuleUnset YearRule = iota
	YearRuleLatest
	YearRuleFixed
)

// Item は、統計表 ID と項目コードの組で項目を識別する。
type Item struct {
	TableID string
	Code    string
}

// Definition は、計算、入力項目、年、換算の倍率、表示の小数桁数を定める。
// 呼び出し中は、Scale と B を含めて変更しないこと。
type Definition struct {
	Op        Operation
	A         Item
	B         *Item
	YearRule  YearRule
	FixedYear int
	SpanYears int
	Scale     *big.Rat
	Decimals  int
}

// ValidateDefinition は、定義をフィールド順に検査し、最初の違反を返す。
// 引数を変更せず、違反は ErrInvalidDefinition を包む *DefinitionError にする。
func ValidateDefinition(d Definition) error {
	if d.Op != OperationValue && d.Op != OperationRatio && d.Op != OperationDiff {
		return definitionError("Op", "must be Value, Ratio or Diff")
	}
	if field := invalidItemField(d.A); field != "" {
		return definitionError("A."+field, "must be nonempty without surrounding whitespace")
	}
	if d.Op == OperationValue {
		if d.B != nil {
			return definitionError("B", "must be nil for Value")
		}
	} else {
		if d.B == nil {
			return definitionError("B", "is required")
		}
		if field := invalidItemField(*d.B); field != "" {
			return definitionError("B."+field, "must be nonempty without surrounding whitespace")
		}
	}
	if d.YearRule != YearRuleLatest && d.YearRule != YearRuleFixed {
		return definitionError("YearRule", "must be Latest or Fixed")
	}
	if (d.YearRule == YearRuleFixed && d.FixedYear <= 0) || (d.YearRule == YearRuleLatest && d.FixedYear != 0) {
		return definitionError("FixedYear", "must be positive for Fixed and zero for Latest")
	}
	if d.SpanYears < 1 || (d.Op != OperationDiff && d.SpanYears != 1) {
		return definitionError("SpanYears", "must be positive and must be one except for Diff")
	}
	if d.Scale == nil || d.Scale.Sign() <= 0 {
		return definitionError("Scale", "must be non-nil and positive")
	}
	if d.Decimals < 0 {
		return definitionError("Decimals", "must be nonnegative")
	}
	return nil
}

func definitionError(field, reason string) error {
	return &DefinitionError{Field: field, Err: fmt.Errorf("%w: %s", ErrInvalidDefinition, reason)}
}

func invalidItemField(item Item) string {
	if !validIdentifier(item.TableID) {
		return "TableID"
	}
	if !validIdentifier(item.Code) {
		return "Code"
	}
	return ""
}

func validIdentifier(s string) bool {
	return s != "" && strings.TrimSpace(s) == s
}
