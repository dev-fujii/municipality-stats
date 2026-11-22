package indicator

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/dev-fujii/municipality-stats/estat"
)

func testRecord(code, area string, year int, raw string) estat.Record {
	status := estat.StatusOK
	switch raw {
	case "***":
		status = estat.StatusNotSurveyed
	case "-":
		status = estat.StatusUnavailable
	case "X":
		status = estat.StatusConfidential
	}
	return estat.Record{TableID: "table", Code: code, Area: area, Year: year, Value: estat.Value{Raw: raw, Status: status}}
}

func checkInvalidCalls(t *testing.T, d Definition, areas []string, records []estat.Record, want error) {
	t.Helper()
	results, err := Calculate(d, areas, records)
	if results != nil || !reflect.DeepEqual(err, want) {
		t.Fatalf("Calculate = %v, %v; want nil, %v", results, err, want)
	}
	years, found, err := SelectYears(d, areas, records)
	if years != (YearRange{}) || found || !reflect.DeepEqual(err, want) {
		t.Fatalf("SelectYears = %+v, %v, %v; want zero, false, %v", years, found, err, want)
	}
}

func checkInputError(t *testing.T, err, class error, index int, field, area string, item Item, year int) {
	t.Helper()
	var ie *InputError
	if !errors.Is(err, class) || !errors.As(err, &ie) {
		t.Fatalf("error = %v, want InputError wrapping %v", err, class)
	}
	if ie.Index != index || ie.Field != field || ie.Area != area || ie.Item != item || ie.Year != year {
		t.Fatalf("InputError = %+v; want index %d, field %s, area %s, item %+v, year %d", ie, index, field, area, item, year)
	}
}

func TestAreas(t *testing.T) {
	tests := []struct {
		name  string
		areas []string
		index int
		field string
		area  string
	}{
		{"nil", nil, -1, "Areas", ""},
		{"empty", []string{}, -1, "Areas", ""},
		{"empty code", []string{"10201", ""}, 1, "Areas[1]", ""},
		{"short", []string{"1020"}, 0, "Areas[0]", "1020"},
		{"long", []string{"102011"}, 0, "Areas[0]", "102011"},
		{"space", []string{" 10201"}, 0, "Areas[0]", " 10201"},
		{"letter", []string{"1020a"}, 0, "Areas[0]", "1020a"},
		{"unicode digits", []string{"１０２０１"}, 0, "Areas[0]", "１０２０１"},
		{"duplicate", []string{"10201", "10424", "10201"}, 2, "Areas[2]", "10201"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDefinition(OperationValue)
			_, _, err := SelectYears(d, tt.areas, nil)
			checkInputError(t, err, ErrInvalidArgument, tt.index, tt.field, tt.area, Item{}, 0)
			checkInvalidCalls(t, d, tt.areas, nil, err)
		})
	}
	if _, err := Calculate(testDefinition(OperationValue), []string{"00000", "01234", "99999"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidRecords(t *testing.T) {
	tests := []struct {
		name   string
		change func(*estat.Record)
		field  string
	}{
		{"empty table", func(r *estat.Record) { r.TableID = "" }, "TableID"},
		{"table space", func(r *estat.Record) { r.TableID = "table " }, "TableID"},
		{"empty code", func(r *estat.Record) { r.Code = "" }, "Code"},
		{"code space", func(r *estat.Record) { r.Code = "\u3000a" }, "Code"},
		{"empty area", func(r *estat.Record) { r.Area = "" }, "Area"},
		{"short area", func(r *estat.Record) { r.Area = "1234" }, "Area"},
		{"long area", func(r *estat.Record) { r.Area = "123456" }, "Area"},
		{"unicode area", func(r *estat.Record) { r.Area = "１２３４５" }, "Area"},
		{"nondigit area", func(r *estat.Record) { r.Area = "12a45" }, "Area"},
		{"zero year", func(r *estat.Record) { r.Year = 0 }, "Year"},
		{"negative year", func(r *estat.Record) { r.Year = -1 }, "Year"},
		{"unset status", func(r *estat.Record) { r.Value.Status = estat.StatusUnset }, "Value.Status"},
		{"unknown status", func(r *estat.Record) { r.Value.Status = 99 }, "Value.Status"},
		{"negative status", func(r *estat.Record) { r.Value.Status = -1 }, "Value.Status"},
	}
	for _, raw := range []string{"1e3", "1/10", "0x10", "1_000", "+0.1", "1,234", " 1", "1 ", "1\n", "", ".1", "1.", "--1", "１２", "***", "-", "X"} {
		tests = append(tests, struct {
			name   string
			change func(*estat.Record)
			field  string
		}{"invalid raw " + raw, func(r *estat.Record) { r.Value.Raw = raw }, "Value.Raw"})
	}
	for _, status := range []estat.Status{estat.StatusNotSurveyed, estat.StatusUnavailable, estat.StatusConfidential} {
		for _, raw := range []string{"", "0", "***", "-", "X"} {
			if testRecord("a", "10201", 2024, raw).Value.Status == status {
				continue
			}
			tests = append(tests, struct {
				name   string
				change func(*estat.Record)
				field  string
			}{fmt.Sprintf("mismatch %d %q", status, raw), func(r *estat.Record) { r.Value = estat.Value{Status: status, Raw: raw} }, "Value.Raw"})
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDefinition(OperationValue)
			r := testRecord("a", "10201", 2024, "1")
			tt.change(&r)
			records := []estat.Record{testRecord("a", "10424", 2023, "2"), r}
			_, _, err := SelectYears(d, []string{"10201"}, records)
			checkInputError(t, err, ErrInvalidRecord, 1, "Records[1]."+tt.field, r.Area, Item{TableID: r.TableID, Code: r.Code}, r.Year)
			checkInvalidCalls(t, d, []string{"10201"}, records, err)
		})
	}
}

func TestRecordsOutsideSelection(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*estat.Record)
	}{
		{"area", func(r *estat.Record) { r.Area = "99999" }},
		{"item", func(r *estat.Record) { r.Code = "other" }},
		{"table", func(r *estat.Record) { r.TableID = "other" }},
		{"year", func(r *estat.Record) { r.Year = 1990 }},
	} {
		t.Run(change.name, func(t *testing.T) {
			d := testDefinition(OperationValue)
			r := testRecord("a", "10201", 2024, "1")
			change.edit(&r)
			for _, duplicate := range []bool{false, true} {
				records := []estat.Record{r, r}
				class, index, field := ErrDuplicateRecord, 1, "Records[1]"
				if !duplicate {
					records = records[:1]
					records[0].Value.Status = estat.StatusUnset
					class, index, field = ErrInvalidRecord, 0, "Records[0].Value.Status"
				}
				_, _, err := SelectYears(d, []string{"10201"}, records)
				checkInputError(t, err, class, index, field, r.Area, Item{TableID: r.TableID, Code: r.Code}, r.Year)
				checkInvalidCalls(t, d, []string{"10201"}, records, err)
			}
		})
	}
}

func TestRecordIdentity(t *testing.T) {
	d := testDefinition(OperationRatio)
	d.B = &Item{TableID: "other", Code: "a"}
	r := testRecord("a", "10201", 2024, "6")
	other := r
	other.TableID, other.Value.Raw = "other", "2"
	got, err := Calculate(d, []string{"10201"}, []estat.Record{r, other})
	if err != nil {
		t.Fatal(err)
	}
	checkResult(t, got[0], "10201", "3", "3.0", &YearRange{2024, 2024}, estat.StatusOK, CalculationStatusOK)
	for _, sameValue := range []bool{true, false} {
		duplicate := r
		if !sameValue {
			duplicate.Value.Raw, duplicate.Unit = "9", "other unit"
		}
		records := []estat.Record{r, other, duplicate}
		_, _, err := SelectYears(d, []string{"10201"}, records)
		checkInputError(t, err, ErrDuplicateRecord, 2, "Records[2]", r.Area, d.A, 2024)
		checkInvalidCalls(t, d, []string{"10201"}, records, err)
	}
}

func TestValidationOrder(t *testing.T) {
	d := Definition{}
	bad := estat.Record{}
	_, _, err := SelectYears(d, nil, []estat.Record{bad})
	checkDefinitionError(t, err, "Op")
	checkInvalidCalls(t, d, nil, []estat.Record{bad}, err)
	d = testDefinition(OperationValue)
	_, _, err = SelectYears(d, nil, []estat.Record{bad})
	checkInputError(t, err, ErrInvalidArgument, -1, "Areas", "", Item{}, 0)
	checkInvalidCalls(t, d, nil, []estat.Record{bad}, err)
}
