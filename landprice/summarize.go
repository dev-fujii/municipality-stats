package landprice

import (
	"fmt"
	"slices"
	"strings"
)

// SummaryState は市区町村ごとの集計の状態を表す。
type SummaryState string

const (
	SummaryOK      SummaryState = "ok"
	SummaryNoPoint SummaryState = "no_point"
)

// Summary は市区町村1つ分の地価の要約を表す。
// 地点が0件のときは State が SummaryNoPoint になり、Median・Min・Max は nil になる。
type Summary struct {
	AreaCode string
	State    SummaryState
	Count    int
	Median   *int64
	Min      *int64
	Max      *int64
	Year     int
}

// Summarize は指定した用途区分の基準地を rollup で統合し、areaCodes の市区町村ごとに集計する。
// 結果は市区町村コードの昇順で返し、入力のスライスやマップは変更しない。
func Summarize(records []Record, useCode string, rollup map[string]string, areaCodes []string) ([]Summary, error) {
	if useCode == "" || strings.TrimSpace(useCode) != useCode {
		return nil, fmt.Errorf("%w: useCode must be non-empty without surrounding whitespace", ErrInvalidArgument)
	}
	if err := validateRollup(rollup); err != nil {
		return nil, err
	}
	sortedAreas, areaSet, err := validateAreaCodes(areaCodes, rollup)
	if err != nil {
		return nil, err
	}

	pricesByArea := make(map[string][]int64, len(sortedAreas))
	var expectedYear int
	var hasYear bool

	for i, r := range records {
		if r.UseCode != useCode {
			continue
		}
		base := fmt.Sprintf("Records[%d]", i)
		where := ReadError{Key: r.AreaCode}
		if r.Year <= 0 {
			return nil, where.fail(base+".Year", invalid("year must be positive, got %d", r.Year))
		}
		if r.Price < 0 {
			return nil, where.fail(base+".Price", invalid("price must not be negative, got %d", r.Price))
		}
		if !fiveDigits(r.AreaCode) {
			return nil, where.fail(base+".AreaCode", invalid("area code must be 5 ASCII digits, got %q", r.AreaCode))
		}
		if !hasYear {
			expectedYear = r.Year
			hasYear = true
		} else if r.Year != expectedYear {
			return nil, where.fail(base+".Year", invalid("mixed survey years %d and %d", expectedYear, r.Year))
		}

		code := r.AreaCode
		if mapped, ok := rollup[code]; ok {
			code = mapped
		}
		if !areaSet[code] {
			w := ReadError{Key: code}
			return nil, w.fail(base+".AreaCode", fmt.Errorf("%w: %s", ErrUnknownArea, code))
		}
		pricesByArea[code] = append(pricesByArea[code], r.Price)
	}

	result := make([]Summary, 0, len(sortedAreas))
	for _, code := range sortedAreas {
		prices := pricesByArea[code]
		if len(prices) == 0 {
			result = append(result, Summary{
				AreaCode: code,
				State:    SummaryNoPoint,
			})
			continue
		}
		slices.Sort(prices)
		minVal := prices[0]
		maxVal := prices[len(prices)-1]
		medVal := integerMedian(prices)
		result = append(result, Summary{
			AreaCode: code,
			State:    SummaryOK,
			Count:    len(prices),
			Median:   &medVal,
			Min:      &minVal,
			Max:      &maxVal,
			Year:     expectedYear,
		})
	}
	return result, nil
}

// integerMedian は昇順に並んだ1件以上の非負整数の中央値を返す。
// 偶数件のときは中央2値の平均を整数に丸め、0.5は切り上げる。
func integerMedian(sorted []int64) int64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	a := sorted[n/2-1]
	b := sorted[n/2]
	diff := b - a
	return a + diff/2 + diff%2
}

func validateRollup(rollup map[string]string) error {
	for from, to := range rollup {
		_, chained := rollup[to]
		if !fiveDigits(from) || !fiveDigits(to) || from == to || chained {
			return fmt.Errorf("%w: invalid, self-referencing or chained rollup %q -> %q", ErrInvalidArgument, from, to)
		}
	}
	return nil
}

func validateAreaCodes(areaCodes []string, rollup map[string]string) ([]string, map[string]bool, error) {
	if len(areaCodes) == 0 {
		return nil, nil, fmt.Errorf("%w: areaCodes must not be empty", ErrInvalidArgument)
	}
	seen := make(map[string]bool, len(areaCodes))
	sorted := make([]string, 0, len(areaCodes))
	for _, code := range areaCodes {
		if !fiveDigits(code) {
			return nil, nil, fmt.Errorf("%w: invalid area code %q", ErrInvalidArgument, code)
		}
		if seen[code] {
			return nil, nil, fmt.Errorf("%w: duplicate area code %q", ErrInvalidArgument, code)
		}
		if _, isRolledUp := rollup[code]; isRolledUp {
			return nil, nil, fmt.Errorf("%w: area code %q is rolled up in rollup map", ErrInvalidArgument, code)
		}
		seen[code] = true
		sorted = append(sorted, code)
	}
	slices.Sort(sorted)
	return sorted, seen, nil
}
