// Package estat は e-Stat API 3.0 の getStatsData を呼び、値と状態の並びに変える。
package estat

import (
	"fmt"
	"regexp"
	"strconv"
)

type Status int

const (
	StatusUnset        Status = iota // 未設定（ゼロ値）。正しい値としては使わない
	StatusOK                         // 数が入っている
	StatusNotSurveyed                // 「***」調査又は集計していない
	StatusUnavailable                // 「-」データが得られない
	StatusConfidential               // 「X」秘匿されている
)

type Value struct {
	Raw    string // e-Stat の "$" そのまま（"102.55" の桁を保つ）
	Status Status
}

var numericValuePattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

var yearTimePattern = regexp.MustCompile(`^[0-9]{4}100000$`)

func parseValue(s string) (Value, error) {
	switch s {
	case "***":
		return Value{Raw: s, Status: StatusNotSurveyed}, nil
	case "-":
		return Value{Raw: s, Status: StatusUnavailable}, nil
	case "X":
		return Value{Raw: s, Status: StatusConfidential}, nil
	}

	if numericValuePattern.MatchString(s) {
		return Value{Raw: s, Status: StatusOK}, nil
	}

	return Value{}, fmt.Errorf("invalid e-Stat value: %q", s)
}

// parseYear は @time の "2023100000" から年の 2023 を取り出す。
// 後ろの6桁が 100000 でない時点（四半期や月）を、年の値として取り込まないようにエラーにする。
func parseYear(s string) (int, error) {
	if !yearTimePattern.MatchString(s) {
		return 0, fmt.Errorf("e-Stat: unexpected @time %q", s)
	}
	return strconv.Atoi(s[:4])
}
