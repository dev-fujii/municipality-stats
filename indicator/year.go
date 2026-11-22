package indicator

import "github.com/dev-fujii/municipality-stats/estat"

// YearRange は、計算に使う予定の連続した年の範囲を表す。
type YearRange struct {
	StartYear int
	EndYear   int
}

// SelectYears は、全入力を検査し、指標に共通する年の範囲を決める。
// Latest の候補年が無ければ、ゼロ値の範囲、false、nil を返す。
func SelectYears(d Definition, areas []string, records []estat.Record) (YearRange, bool, error) {
	byArea, err := validateInput(d, areas, records)
	if err != nil {
		return YearRange{}, false, err
	}
	years, found := selectYears(d, byArea)
	return years, found, nil
}

func selectYears(d Definition, byArea map[string][]inputRecord) (YearRange, bool) {
	year := d.FixedYear
	if d.YearRule == YearRuleLatest {
		candidates := make(map[int]uint8)
		for _, rows := range byArea {
			for _, r := range rows {
				if r.status == estat.StatusNotSurveyed {
					continue
				}
				if r.item == d.A {
					candidates[r.year] |= 1
				}
				if d.B != nil && r.item == *d.B {
					candidates[r.year] |= 2
				}
			}
		}
		var required uint8 = 1
		if d.B != nil {
			required |= 2
		}
		for candidate, present := range candidates {
			if present == required && candidate > year {
				year = candidate
			}
		}
		if year == 0 {
			return YearRange{}, false
		}
	}
	return YearRange{StartYear: year - (d.SpanYears - 1), EndYear: year}, true
}
