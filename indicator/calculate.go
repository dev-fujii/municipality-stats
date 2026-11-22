package indicator

import (
	"math/big"

	"github.com/dev-fujii/municipality-stats/estat"
)

// CalculationStatus は、必要行の有無と0除算による計算の状態を表す。
type CalculationStatus int

// CalculationStatusUnset 以降は、未設定、年不足と0除算なし、0除算、年不足を区別する。
const (
	CalculationStatusUnset CalculationStatus = iota
	CalculationStatusOK
	CalculationStatusDivisionByZero
	CalculationStatusInsufficientYears
)

// Result は、地域ごとの値、表示、予定した年の範囲と2種類の状態を持つ。
// Value と Display は、InputStatus と CalculationStatus がともに OK のときだけ得られる。
// Value は倍率を掛けたあとの丸め前の値で、各結果の Value と Years は独立して変更できる。
type Result struct {
	Area              string
	Value             *big.Rat
	Display           string
	Years             *YearRange
	InputStatus       estat.Status
	CalculationStatus CalculationStatus
}

// Calculate は、全入力を検査し、共通の年で計算した結果を areas の順に返す。
// 引数を変更せず、不正な入力では途中の結果を返さず nil とエラーを返す。
func Calculate(d Definition, areas []string, records []estat.Record) ([]Result, error) {
	byArea, err := validateInput(d, areas, records)
	if err != nil {
		return nil, err
	}
	years, found := selectYears(d, byArea)
	results := make([]Result, len(areas))
	for i, area := range areas {
		if !found {
			results[i] = Result{Area: area, InputStatus: estat.StatusOK, CalculationStatus: CalculationStatusInsufficientYears}
			continue
		}
		results[i] = calculateArea(d, area, years, byArea[area])
	}
	return results, nil
}

func calculateArea(d Definition, area string, years YearRange, rows []inputRecord) Result {
	result := Result{Area: area, Years: &years, InputStatus: estat.StatusOK, CalculationStatus: CalculationStatusOK}
	countA, countB := 0, 0
	bIsZero := false
	for _, r := range rows {
		if r.year < years.StartYear || r.year > years.EndYear {
			continue
		}
		result.InputStatus = strongerStatus(result.InputStatus, r.status)
		if r.item == d.A {
			countA++
		}
		if d.B != nil && r.item == *d.B {
			countB++
			if r.value != nil {
				bIsZero = r.value.Sign() == 0
			}
		}
	}
	// 重複を入口で除くので、件数で連続する全整数年の充足を判定できる。
	// 年を順にたどらず、非常に長い期間や int の最大年でも同じ処理にする。
	if countA != d.SpanYears || (d.B != nil && countB != d.SpanYears) {
		result.CalculationStatus = CalculationStatusInsufficientYears
		return result
	}
	if d.Op == OperationRatio && bIsZero {
		result.CalculationStatus = CalculationStatusDivisionByZero
		return result
	}
	if result.InputStatus != estat.StatusOK {
		return result
	}
	a, b := new(big.Rat), new(big.Rat)
	for _, r := range rows {
		if r.year < years.StartYear || r.year > years.EndYear {
			continue
		}
		if r.item == d.A {
			a.Add(a, r.value)
		}
		if d.B != nil && r.item == *d.B {
			b.Add(b, r.value)
		}
	}
	switch d.Op {
	case OperationRatio:
		a.Quo(a, b)
	case OperationDiff:
		a.Sub(a, b)
	}
	result.Value = a.Mul(a, d.Scale)
	result.Display = formatValue(result.Value, d.Decimals)
	return result
}

func strongerStatus(a, b estat.Status) estat.Status {
	if b > a {
		return b
	}
	return a
}
