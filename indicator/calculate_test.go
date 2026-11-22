package indicator

import (
	"fmt"
	"math/big"
	"reflect"
	"slices"
	"testing"

	"github.com/dev-fujii/municipality-stats/estat"
)

func fmtName(op Operation, detail string) string {
	return fmt.Sprintf("%d/%s", op, detail)
}

func testRat(t *testing.T, s string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("invalid test rational %q", s)
	}
	return r
}

func checkResult(t *testing.T, got Result, area, value, display string, years *YearRange, input estat.Status, calculation CalculationStatus) {
	t.Helper()
	if got.Area != area || got.Display != display || !reflect.DeepEqual(got.Years, years) || got.InputStatus != input || got.CalculationStatus != calculation {
		t.Fatalf("result = %+v; want area %s, display %q, years %+v, input %d, calculation %d", got, area, display, years, input, calculation)
	}
	if value == "" {
		if got.Value != nil {
			t.Fatalf("value = %s, want nil", got.Value)
		}
	} else if got.Value == nil || got.Value.Cmp(testRat(t, value)) != 0 {
		t.Fatalf("value = %v, want %s", got.Value, value)
	}
}

func TestCalculate(t *testing.T) {
	tests := []struct {
		name    string
		op      Operation
		scale   *big.Rat
		span    int
		a       []string
		b       []string
		value   string
		display string
	}{
		{"value", OperationValue, big.NewRat(10, 1), 1, []string{"12.35"}, nil, "247/2", "123.5"},
		{"zero", OperationValue, big.NewRat(1, 1), 1, []string{"0"}, nil, "0", "0.0"},
		{"ratio", OperationRatio, big.NewRat(1, 10), 1, []string{"10"}, []string{"3"}, "1/3", "0.3"},
		{"ratio round once", OperationRatio, big.NewRat(1, 10), 1, []string{"249"}, []string{"20"}, "249/200", "1.2"},
		{"ratio zero numerator", OperationRatio, big.NewRat(1, 1), 1, []string{"0"}, []string{"3"}, "0", "0.0"},
		{"ratio negative denominator", OperationRatio, big.NewRat(1, 1), 1, []string{"5"}, []string{"-4"}, "-5/4", "-1.3"},
		{"diff", OperationDiff, big.NewRat(1, 1), 3, []string{"5", "8", "9"}, []string{"2", "3", "4"}, "13", "13.0"},
		{"diff single year", OperationDiff, big.NewRat(1, 1), 1, []string{"5"}, []string{"2"}, "3", "3.0"},
		{"diff round once", OperationDiff, big.NewRat(10, 1), 3, []string{"0.06", "0.06", "0.06"}, []string{"0.02", "0.02", "0.02"}, "6/5", "1.2"},
		{"negative diff", OperationDiff, big.NewRat(1, 1), 2, []string{"1", "2"}, []string{"3", "4"}, "-4", "-4.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDefinition(tt.op)
			d.Scale, d.SpanYears = tt.scale, tt.span
			start := 2024 - (tt.span - 1)
			records := make([]estat.Record, 0, len(tt.a)+len(tt.b))
			for i, raw := range tt.a {
				records = append(records, testRecord("a", "10201", start+i, raw))
			}
			for i, raw := range tt.b {
				records = append(records, testRecord("b", "10201", start+i, raw))
			}
			got, err := Calculate(d, []string{"10201"}, records)
			if err != nil || len(got) != 1 {
				t.Fatalf("Calculate = %+v, %v", got, err)
			}
			checkResult(t, got[0], "10201", tt.value, tt.display, &YearRange{start, 2024}, estat.StatusOK, CalculationStatusOK)
		})
	}
}

func TestCalculateInputStatuses(t *testing.T) {
	inputs := []string{"2", "***", "-", "X"}
	for i, a := range inputs {
		for j, b := range inputs {
			for _, op := range []Operation{OperationRatio, OperationDiff} {
				t.Run(fmtName(op, a+"/"+b), func(t *testing.T) {
					d := testDefinition(op)
					got, err := Calculate(d, []string{"10201"}, []estat.Record{
						testRecord("a", "10201", 2024, a), testRecord("b", "10201", 2024, b),
					})
					if err != nil {
						t.Fatal(err)
					}
					status := []estat.Status{estat.StatusOK, estat.StatusNotSurveyed, estat.StatusUnavailable, estat.StatusConfidential}[max(i, j)]
					value, display := "", ""
					if status == estat.StatusOK {
						value, display = "1", "1.0"
						if op == OperationDiff {
							value, display = "0", "0.0"
						}
					}
					checkResult(t, got[0], "10201", value, display, &YearRange{2024, 2024}, status, CalculationStatusOK)
				})
			}
		}
	}
	for _, raw := range inputs[1:] {
		d := testDefinition(OperationValue)
		r := testRecord("a", "10201", 2024, raw)
		got, err := Calculate(d, []string{"10201"}, []estat.Record{r})
		if err != nil {
			t.Fatal(err)
		}
		checkResult(t, got[0], "10201", "", "", &YearRange{2024, 2024}, r.Value.Status, CalculationStatusOK)
	}
}

func TestCalculateMissingAndZero(t *testing.T) {
	tests := []struct {
		name    string
		op      Operation
		rule    YearRule
		fixed   int
		span    int
		records []estat.Record
		years   YearRange
		input   estat.Status
		status  CalculationStatus
	}{
		{"value absent", OperationValue, YearRuleFixed, 2024, 1, nil, YearRange{2024, 2024}, estat.StatusOK, CalculationStatusInsufficientYears},
		{"ratio a absent b confidential", OperationRatio, YearRuleFixed, 2024, 1, []estat.Record{
			testRecord("b", "10201", 2024, "X"),
		}, YearRange{2024, 2024}, estat.StatusConfidential, CalculationStatusInsufficientYears},
		{"ratio b absent a confidential", OperationRatio, YearRuleFixed, 2024, 1, []estat.Record{
			testRecord("a", "10201", 2024, "X"),
		}, YearRange{2024, 2024}, estat.StatusConfidential, CalculationStatusInsufficientYears},
		{"ratio a absent b zero", OperationRatio, YearRuleFixed, 2024, 1, []estat.Record{
			testRecord("b", "10201", 2024, "0"),
		}, YearRange{2024, 2024}, estat.StatusOK, CalculationStatusInsufficientYears},
		{"zero denominator", OperationRatio, YearRuleFixed, 2024, 1, []estat.Record{
			testRecord("a", "10201", 2024, "1"), testRecord("b", "10201", 2024, "0"),
		}, YearRange{2024, 2024}, estat.StatusOK, CalculationStatusDivisionByZero},
		{"negative zero denominator", OperationRatio, YearRuleFixed, 2024, 1, []estat.Record{
			testRecord("a", "10201", 2024, "1"), testRecord("b", "10201", 2024, "-0.0"),
		}, YearRange{2024, 2024}, estat.StatusOK, CalculationStatusDivisionByZero},
		{"confidential and zero", OperationRatio, YearRuleFixed, 2024, 1, []estat.Record{
			testRecord("a", "10201", 2024, "X"), testRecord("b", "10201", 2024, "0"),
		}, YearRange{2024, 2024}, estat.StatusConfidential, CalculationStatusDivisionByZero},
		{"symbol denominator", OperationRatio, YearRuleFixed, 2024, 1, []estat.Record{
			testRecord("a", "10201", 2024, "0"), testRecord("b", "10201", 2024, "-"),
		}, YearRange{2024, 2024}, estat.StatusUnavailable, CalculationStatusOK},
		{"diff missing middle and confidential", OperationDiff, YearRuleLatest, 0, 3, []estat.Record{
			testRecord("a", "10201", 2022, "1"), testRecord("b", "10201", 2022, "1"),
			testRecord("a", "10201", 2024, "X"), testRecord("b", "10201", 2024, "1"),
		}, YearRange{2022, 2024}, estat.StatusConfidential, CalculationStatusInsufficientYears},
		{"diff earlier not surveyed", OperationDiff, YearRuleLatest, 0, 3, []estat.Record{
			testRecord("a", "10201", 2022, "***"), testRecord("b", "10201", 2022, "1"),
			testRecord("a", "10201", 2023, "1"), testRecord("b", "10201", 2023, "1"),
			testRecord("a", "10201", 2024, "1"), testRecord("b", "10201", 2024, "1"),
		}, YearRange{2022, 2024}, estat.StatusNotSurveyed, CalculationStatusOK},
		{"diff start before positive years", OperationDiff, YearRuleLatest, 0, 3, []estat.Record{
			testRecord("a", "10201", 1, "X"), testRecord("b", "10201", 1, "1"),
		}, YearRange{-1, 1}, estat.StatusConfidential, CalculationStatusInsufficientYears},
		{"diff fixed no rows", OperationDiff, YearRuleFixed, 2024, 3, nil, YearRange{2022, 2024}, estat.StatusOK, CalculationStatusInsufficientYears},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDefinition(tt.op)
			d.YearRule, d.FixedYear, d.SpanYears = tt.rule, tt.fixed, tt.span
			got, err := Calculate(d, []string{"10201"}, tt.records)
			if err != nil {
				t.Fatal(err)
			}
			checkResult(t, got[0], "10201", "", "", &tt.years, tt.input, tt.status)
		})
	}
}

func TestDiffAggregatesWholeRange(t *testing.T) {
	for _, symbols := range [][]string{{"X", "-", "***"}, {"***", "X", "-"}, {"-", "***", "X"}} {
		for _, missing := range []bool{false, true} {
			d := testDefinition(OperationDiff)
			d.SpanYears = 3
			var records []estat.Record
			for i, raw := range symbols {
				records = append(records, testRecord("a", "10201", 2022+i, raw))
				if !missing || i != 1 {
					records = append(records, testRecord("b", "10201", 2022+i, "1"))
				}
			}
			got, err := Calculate(d, []string{"10201"}, records)
			if err != nil {
				t.Fatal(err)
			}
			status := CalculationStatusOK
			if missing {
				status = CalculationStatusInsufficientYears
			}
			checkResult(t, got[0], "10201", "", "", &YearRange{2022, 2024}, estat.StatusConfidential, status)
		}
	}
}

func TestCalculateAreasAndOrder(t *testing.T) {
	d := testDefinition(OperationValue)
	d.YearRule, d.FixedYear = YearRuleLatest, 0
	areas := []string{"10424", "10201", "10425"}
	records := []estat.Record{
		testRecord("a", "10201", 2023, "X"), testRecord("a", "10201", 2024, "2"),
		testRecord("a", "10424", 2023, "3"), testRecord("b", "10424", 2025, "X"),
		testRecord("a", "99999", 2025, "4"),
	}
	first, err := Calculate(d, areas, records)
	if err != nil || len(first) != len(areas) {
		t.Fatalf("Calculate = %+v, %v", first, err)
	}
	checkResult(t, first[0], "10424", "", "", &YearRange{2024, 2024}, estat.StatusOK, CalculationStatusInsufficientYears)
	checkResult(t, first[1], "10201", "2", "2.0", &YearRange{2024, 2024}, estat.StatusOK, CalculationStatusOK)
	checkResult(t, first[2], "10425", "", "", &YearRange{2024, 2024}, estat.StatusOK, CalculationStatusInsufficientYears)
	slices.Reverse(records)
	second, err := Calculate(d, areas, records)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("reversed Calculate = %+v, %v; want %+v", second, err, first)
	}
}

func TestCalculateFixedYear(t *testing.T) {
	d := testDefinition(OperationValue)
	d.FixedYear = 2023
	got, err := Calculate(d, []string{"10201", "10424"}, []estat.Record{
		testRecord("a", "10201", 2023, "2"), testRecord("a", "10201", 2024, "3"),
		testRecord("a", "10424", 2024, "X"),
	})
	if err != nil {
		t.Fatal(err)
	}
	checkResult(t, got[0], "10201", "2", "2.0", &YearRange{2023, 2023}, estat.StatusOK, CalculationStatusOK)
	checkResult(t, got[1], "10424", "", "", &YearRange{2023, 2023}, estat.StatusOK, CalculationStatusInsufficientYears)
}

func TestCalculateSameItem(t *testing.T) {
	for _, op := range []Operation{OperationRatio, OperationDiff} {
		d := testDefinition(op)
		d.B = &d.A
		for _, raw := range []string{"2", "0", "X"} {
			got, err := Calculate(d, []string{"10201"}, []estat.Record{testRecord("a", "10201", 2024, raw)})
			if err != nil {
				t.Fatal(err)
			}
			input, status, value, display := estat.StatusOK, CalculationStatusOK, "0", "0.0"
			if op == OperationRatio {
				value, display = "1", "1.0"
				if raw == "0" {
					status, value, display = CalculationStatusDivisionByZero, "", ""
				}
			}
			if raw == "X" {
				input, value, display = estat.StatusConfidential, "", ""
			}
			checkResult(t, got[0], "10201", value, display, &YearRange{2024, 2024}, input, status)
		}
	}
}

func TestArgumentsAndResultsIndependent(t *testing.T) {
	for _, op := range []Operation{OperationValue, OperationRatio, OperationDiff} {
		t.Run(fmtName(op, "independent"), func(t *testing.T) {
			d := testDefinition(op)
			d.Scale = big.NewRat(2, 3)
			areas := []string{"10424", "10201"}
			records := []estat.Record{
				testRecord("a", "10201", 2024, "3"), testRecord("a", "10424", 2024, "3"),
				testRecord("b", "10201", 2024, "1"), testRecord("b", "10424", 2024, "1"),
			}
			records[0].Unit = "uninterpreted unit"
			beforeAreas, beforeRecords := slices.Clone(areas), slices.Clone(records)
			beforeScale := new(big.Rat).Set(d.Scale)
			var beforeB Item
			if d.B != nil {
				beforeB = *d.B
			}
			if err := ValidateDefinition(d); err != nil {
				t.Fatal(err)
			}
			if _, _, err := SelectYears(d, areas, records); err != nil {
				t.Fatal(err)
			}
			got, err := Calculate(d, areas, records)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(areas, beforeAreas) || !slices.Equal(records, beforeRecords) || d.Scale.Cmp(beforeScale) != 0 || (d.B != nil && *d.B != beforeB) {
				t.Fatal("input was changed")
			}
			otherValue := new(big.Rat).Set(got[1].Value)
			got[0].Value.SetInt64(99)
			got[0].Years.StartYear, got[0].Years.EndYear = 1, 2
			if got[1].Value.Cmp(otherValue) != 0 || *got[1].Years != (YearRange{2024, 2024}) || d.Scale.Cmp(beforeScale) != 0 {
				t.Fatal("results share mutable values")
			}
			again, err := Calculate(d, areas, records)
			if err != nil || again[0].Value.Cmp(otherValue) != 0 || *again[0].Years != (YearRange{2024, 2024}) {
				t.Fatalf("later Calculate = %+v, %v", again, err)
			}
		})
	}
}

func TestCalculateExtremeYears(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tt := range []struct {
		fixed int
		span  int
		start int
		value string
	}{
		{maxInt, 1, maxInt, "1"},
		{maxInt, maxInt, 1, ""},
		{1, maxInt, 2 - maxInt, ""},
	} {
		d := testDefinition(OperationDiff)
		d.FixedYear, d.SpanYears = tt.fixed, tt.span
		got, err := Calculate(d, []string{"10201"}, []estat.Record{
			testRecord("a", "10201", tt.fixed, "2"), testRecord("b", "10201", tt.fixed, "1"),
		})
		if err != nil {
			t.Fatal(err)
		}
		display, status := "1.0", CalculationStatusOK
		if tt.value == "" {
			display, status = "", CalculationStatusInsufficientYears
		}
		checkResult(t, got[0], "10201", tt.value, display, &YearRange{tt.start, tt.fixed}, estat.StatusOK, status)
	}
}

func ExampleCalculate() {
	d := Definition{
		Op: OperationRatio, A: Item{TableID: "table", Code: "a"}, B: &Item{TableID: "table", Code: "b"},
		YearRule: YearRuleLatest, SpanYears: 1, Scale: big.NewRat(1, 10), Decimals: 1,
	}
	records := []estat.Record{
		{TableID: "table", Code: "a", Area: "10201", Year: 2024, Value: estat.Value{Raw: "10", Status: estat.StatusOK}},
		{TableID: "table", Code: "b", Area: "10201", Year: 2024, Value: estat.Value{Raw: "3", Status: estat.StatusOK}},
	}
	results, err := Calculate(d, []string{"10201", "10424"}, records)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, r := range results {
		if r.Years == nil {
			fmt.Printf("%s: no candidate year\n", r.Area)
			continue
		}
		if r.InputStatus != estat.StatusOK || r.CalculationStatus != CalculationStatusOK {
			fmt.Printf("%s: %d-%d input=%d calculation=%d\n", r.Area, r.Years.StartYear, r.Years.EndYear, r.InputStatus, r.CalculationStatus)
			continue
		}
		fmt.Printf("%s: %d-%d %s\n", r.Area, r.Years.StartYear, r.Years.EndYear, r.Display)
	}
	// Output:
	// 10201: 2024-2024 0.3
	// 10424: 2024-2024 input=1 calculation=3
}
