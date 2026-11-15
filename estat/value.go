// Package estat は e-Stat API 3.0 の getStatsData を呼び、値と状態の並びに変える。
package estat

import (
	"fmt"
	"regexp"
)

type Status int

const (
	StatusOK           Status = iota // 数が入っている
	StatusNotSurveyed                // 「***」調査又は集計していない
	StatusUnavailable                // 「-」データが得られない
	StatusConfidential               // 「X」秘匿されている
)

type Value struct {
	Raw    string // e-Stat の "$" そのまま（"102.55" の桁を保つ）
	Status Status
}

var numericValuePattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

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
