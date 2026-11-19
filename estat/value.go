// Package estat は e-Stat API 3.0 の getStatsData を呼び、値と状態の並びに変える。
package estat

import (
	"fmt"
	"regexp"
	"strconv"
)

// Status は、e-Stat の値に数があるか、どの理由で無いかを表す。
// ゼロ値を未設定にし、初期化を忘れた値を数が入っていると読まないようにする。
type Status int

// StatusUnset 以降の定数は、未設定、数値、3種類の記号を区別する。
// 記号を別々の状態にし、あとから値が無い理由を区別できるようにする。
const (
	StatusUnset        Status = iota // 未設定（ゼロ値）。正しい値としては使わない
	StatusOK                         // 数が入っている
	StatusNotSurveyed                // 「***」調査又は集計していない
	StatusUnavailable                // 「-」データが得られない
	StatusConfidential               // 「X」秘匿されている
)

// Value は、e-Stat の元の文字列と、その値の状態を持つ。
// Raw は float64 にせず文字列で持ち、102.55 のような値の桁を丸めずに保つ。
type Value struct {
	Raw    string // e-Stat の "$" そのまま（"102.55" の桁を保つ）
	Status Status
}

// numericValuePattern は、受け付ける数の文字列の形を定める。
// 「-」だけは記号として除き、人口増減率のような負の数は受け付ける。
var numericValuePattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// yearTimePattern は、社会・人口統計体系の年を表す @time の形を定める。
// 四半期や月の値を年として取り込まないよう、10桁の数字で末尾が 100000 のものに限る。
var yearTimePattern = regexp.MustCompile(`^[0-9]{4}100000$`)

// parseValue は、値の文字列を元の文字列と状態に変える。
// API の変化や読み漏れに気づけるよう、数と既知の3記号以外はエラーにする。
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
