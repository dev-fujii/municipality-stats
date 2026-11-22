package indicator

import (
	"testing"

	"github.com/dev-fujii/municipality-stats/estat"
)

func TestSelectYears(t *testing.T) {
	tests := []struct {
		name    string
		op      Operation
		rule    YearRule
		fixed   int
		span    int
		records []estat.Record
		want    YearRange
		found   bool
	}{
		{"fixed ignores newer", OperationValue, YearRuleFixed, 2023, 1, []estat.Record{
			testRecord("a", "10201", 2023, "1"), testRecord("a", "10201", 2024, "2"),
		}, YearRange{2023, 2023}, true},
		{"fixed no rows", OperationValue, YearRuleFixed, 2024, 1, nil, YearRange{2024, 2024}, true},
		{"fixed diff nonpositive start", OperationDiff, YearRuleFixed, 1, 3, nil, YearRange{-1, 1}, true},
		{"common year", OperationRatio, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("a", "10201", 2023, "1"), testRecord("a", "10201", 2024, "2"),
			testRecord("b", "10201", 2022, "3"), testRecord("b", "10201", 2023, "4"),
		}, YearRange{2023, 2023}, true},
		{"different areas and outside area", OperationRatio, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("a", "10201", 2024, "1"), testRecord("b", "10424", 2024, "2"),
			testRecord("a", "99999", 2025, "1"), testRecord("b", "99999", 2025, "2"),
		}, YearRange{2024, 2024}, true},
		{"outside item", OperationValue, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("a", "10201", 2023, "1"), testRecord("b", "10201", 2025, "2"),
		}, YearRange{2023, 2023}, true},
		{"survey gaps", OperationValue, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("a", "10201", 2018, "1"), testRecord("a", "10201", 2023, "2"),
		}, YearRange{2023, 2023}, true},
		{"diff missing intermediate year", OperationDiff, YearRuleLatest, 0, 3, []estat.Record{
			testRecord("a", "10201", 2021, "1"), testRecord("b", "10201", 2021, "1"),
			testRecord("a", "10201", 2022, "1"), testRecord("b", "10201", 2022, "1"),
			testRecord("a", "10201", 2024, "1"), testRecord("b", "10201", 2024, "1"),
		}, YearRange{2022, 2024}, true},
		{"nil rows", OperationValue, YearRuleLatest, 0, 1, nil, YearRange{}, false},
		{"empty rows", OperationValue, YearRuleLatest, 0, 1, []estat.Record{}, YearRange{}, false},
		{"only outside areas", OperationValue, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("a", "99999", 2024, "1"),
		}, YearRange{}, false},
		{"only outside items", OperationValue, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("other", "10201", 2024, "1"),
		}, YearRange{}, false},
		{"only not surveyed", OperationValue, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("a", "10201", 2023, "***"), testRecord("a", "10424", 2024, "***"),
		}, YearRange{}, false},
		{"no common year", OperationRatio, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("a", "10201", 2024, "1"), testRecord("b", "10201", 2023, "1"),
		}, YearRange{}, false},
		{"missing b", OperationRatio, YearRuleLatest, 0, 1, []estat.Record{
			testRecord("a", "10201", 2024, "1"),
		}, YearRange{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDefinition(tt.op)
			d.YearRule, d.FixedYear, d.SpanYears = tt.rule, tt.fixed, tt.span
			areas := []string{"10424", "10201"}
			years, found, err := SelectYears(d, areas, tt.records)
			if err != nil || years != tt.want || found != tt.found {
				t.Fatalf("SelectYears = %+v, %v, %v; want %+v, %v", years, found, err, tt.want, tt.found)
			}
			got, err := Calculate(d, areas, tt.records)
			if err != nil || len(got) != len(areas) {
				t.Fatalf("Calculate = %+v, %v", got, err)
			}
			for i, r := range got {
				if r.Area != areas[i] {
					t.Fatalf("area = %s, want %s", r.Area, areas[i])
				}
				if !tt.found {
					checkResult(t, r, areas[i], "", "", nil, estat.StatusOK, CalculationStatusInsufficientYears)
				} else if r.Years == nil || *r.Years != tt.want {
					t.Fatalf("result years = %+v, want %+v", r.Years, tt.want)
				}
			}
		})
	}
}

func TestLatestNotSurveyedAndAbsent(t *testing.T) {
	for _, op := range []Operation{OperationValue, OperationRatio, OperationDiff} {
		for _, code := range []string{"a", "b"} {
			if op == OperationValue && code == "b" {
				continue
			}
			t.Run(fmtName(op, code), func(t *testing.T) {
				d := testDefinition(op)
				d.YearRule, d.FixedYear = YearRuleLatest, 0
				records := []estat.Record{testRecord("a", "10201", 2023, "2")}
				if d.B != nil {
					records = append(records, testRecord("b", "10201", 2023, "1"))
					other := "a"
					if code == "a" {
						other = "b"
					}
					records = append(records, testRecord(other, "10201", 2024, "3"))
				}
				withSymbols := append(append([]estat.Record(nil), records...),
					testRecord(code, "10201", 2024, "***"), testRecord(code, "10424", 2024, "***"))
				for _, input := range [][]estat.Record{records, withSymbols} {
					years, found, err := SelectYears(d, []string{"10201", "10424"}, input)
					if err != nil || !found || years != (YearRange{2023, 2023}) {
						t.Fatalf("SelectYears = %+v, %v, %v", years, found, err)
					}
				}
			})
		}
	}
}

func TestLatestSymbols(t *testing.T) {
	for _, op := range []Operation{OperationValue, OperationRatio, OperationDiff} {
		for _, symbol := range []string{"-", "X"} {
			t.Run(fmtName(op, symbol), func(t *testing.T) {
				d := testDefinition(op)
				d.YearRule, d.FixedYear = YearRuleLatest, 0
				records := []estat.Record{testRecord("a", "10201", 2023, "2"), testRecord("a", "10201", 2024, symbol)}
				if d.B != nil {
					records = append(records, testRecord("b", "10201", 2023, "1"), testRecord("b", "10201", 2024, symbol))
				}
				got, err := Calculate(d, []string{"10201"}, records)
				if err != nil {
					t.Fatal(err)
				}
				checkResult(t, got[0], "10201", "", "", &YearRange{2024, 2024}, records[1].Value.Status, CalculationStatusOK)
			})
		}
	}
}

func TestSameItemYears(t *testing.T) {
	for _, op := range []Operation{OperationRatio, OperationDiff} {
		d := testDefinition(op)
		d.B, d.YearRule, d.FixedYear = &d.A, YearRuleLatest, 0
		years, found, err := SelectYears(d, []string{"10201"}, []estat.Record{testRecord("a", "10201", 2024, "2")})
		if err != nil || !found || years != (YearRange{2024, 2024}) {
			t.Fatalf("SelectYears = %+v, %v, %v", years, found, err)
		}
	}
}

func TestLatestSurveyedInAnyArea(t *testing.T) {
	d := testDefinition(OperationRatio)
	d.YearRule, d.FixedYear = YearRuleLatest, 0
	d.B = &Item{TableID: "other", Code: "a"}
	other := testRecord("a", "10424", 2024, "X")
	other.TableID = "other"
	missingSurvey := testRecord("a", "10201", 2024, "***")
	missingSurvey.TableID = "other"
	records := []estat.Record{
		testRecord("a", "10201", 2024, "-"), testRecord("a", "10424", 2024, "***"),
		other, missingSurvey, testRecord("a", "10201", 2025, "1"),
	}
	got, err := Calculate(d, []string{"10201", "10424"}, records)
	if err != nil {
		t.Fatal(err)
	}
	checkResult(t, got[0], "10201", "", "", &YearRange{2024, 2024}, estat.StatusUnavailable, CalculationStatusOK)
	checkResult(t, got[1], "10424", "", "", &YearRange{2024, 2024}, estat.StatusConfidential, CalculationStatusOK)
}
