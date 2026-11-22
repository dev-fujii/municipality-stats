package indicator

import (
	"errors"
	"math/big"
	"testing"
)

func testDefinition(op Operation) Definition {
	d := Definition{Op: op, A: Item{TableID: "table", Code: "a"}, YearRule: YearRuleFixed,
		FixedYear: 2024, SpanYears: 1, Scale: big.NewRat(1, 1), Decimals: 1}
	if op != OperationValue {
		d.B = &Item{TableID: "table", Code: "b"}
	}
	return d
}

func TestValidateDefinition(t *testing.T) {
	tests := []struct {
		name   string
		op     Operation
		change func(*Definition)
		field  string
	}{
		{"value", OperationValue, func(*Definition) {}, ""},
		{"ratio", OperationRatio, func(*Definition) {}, ""},
		{"diff", OperationDiff, func(d *Definition) { d.SpanYears = 3 }, ""},
		{"same item", OperationRatio, func(d *Definition) { d.B = &d.A }, ""},
		{"same item diff", OperationDiff, func(d *Definition) { d.B = &d.A }, ""},
		{"latest", OperationValue, func(d *Definition) { d.YearRule, d.FixedYear = YearRuleLatest, 0 }, ""},
		{"positive year", OperationValue, func(d *Definition) { d.FixedYear = 1 }, ""},
		{"zero decimals", OperationValue, func(d *Definition) { d.Decimals = 0 }, ""},
		{"many decimals", OperationValue, func(d *Definition) { d.Decimals = 30 }, ""},
		{"internal whitespace", OperationValue, func(d *Definition) { d.A.Code = "a b" }, ""},
		{"unset operation", OperationValue, func(d *Definition) { d.Op = OperationUnset }, "Op"},
		{"unknown operation", OperationValue, func(d *Definition) { d.Op = 99 }, "Op"},
		{"negative operation", OperationValue, func(d *Definition) { d.Op = -1 }, "Op"},
		{"empty a table", OperationValue, func(d *Definition) { d.A.TableID = "" }, "A.TableID"},
		{"empty a code", OperationValue, func(d *Definition) { d.A.Code = "" }, "A.Code"},
		{"a table whitespace", OperationValue, func(d *Definition) { d.A.TableID = " table" }, "A.TableID"},
		{"a code whitespace", OperationValue, func(d *Definition) { d.A.Code = "a\u3000" }, "A.Code"},
		{"extra b", OperationValue, func(d *Definition) { d.B = &Item{} }, "B"},
		{"missing ratio b", OperationRatio, func(d *Definition) { d.B = nil }, "B"},
		{"missing diff b", OperationDiff, func(d *Definition) { d.B = nil }, "B"},
		{"empty b table", OperationRatio, func(d *Definition) { d.B.TableID = "" }, "B.TableID"},
		{"empty b code", OperationRatio, func(d *Definition) { d.B.Code = "" }, "B.Code"},
		{"b table whitespace", OperationRatio, func(d *Definition) { d.B.TableID = "table\n" }, "B.TableID"},
		{"b code whitespace", OperationDiff, func(d *Definition) { d.B.Code = "\tb" }, "B.Code"},
		{"unset year rule", OperationValue, func(d *Definition) { d.YearRule = YearRuleUnset }, "YearRule"},
		{"unknown year rule", OperationValue, func(d *Definition) { d.YearRule = 99 }, "YearRule"},
		{"negative year rule", OperationValue, func(d *Definition) { d.YearRule = -1 }, "YearRule"},
		{"fixed zero", OperationValue, func(d *Definition) { d.FixedYear = 0 }, "FixedYear"},
		{"fixed negative", OperationValue, func(d *Definition) { d.FixedYear = -1 }, "FixedYear"},
		{"latest nonzero", OperationValue, func(d *Definition) { d.YearRule = YearRuleLatest }, "FixedYear"},
		{"latest negative", OperationValue, func(d *Definition) { d.YearRule, d.FixedYear = YearRuleLatest, -1 }, "FixedYear"},
		{"zero span", OperationDiff, func(d *Definition) { d.SpanYears = 0 }, "SpanYears"},
		{"negative span", OperationDiff, func(d *Definition) { d.SpanYears = -1 }, "SpanYears"},
		{"value multiple years", OperationValue, func(d *Definition) { d.SpanYears = 2 }, "SpanYears"},
		{"ratio multiple years", OperationRatio, func(d *Definition) { d.SpanYears = 2 }, "SpanYears"},
		{"nil scale", OperationValue, func(d *Definition) { d.Scale = nil }, "Scale"},
		{"zero scale", OperationValue, func(d *Definition) { d.Scale = new(big.Rat) }, "Scale"},
		{"negative scale", OperationValue, func(d *Definition) { d.Scale = big.NewRat(-1, 10) }, "Scale"},
		{"negative decimals", OperationValue, func(d *Definition) { d.Decimals = -1 }, "Decimals"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDefinition(tt.op)
			tt.change(&d)
			err := ValidateDefinition(d)
			if tt.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			checkDefinitionError(t, err, tt.field)
			checkInvalidCalls(t, d, []string{"10201"}, nil, err)
		})
	}
}

func checkDefinitionError(t *testing.T, err error, field string) {
	t.Helper()
	var de *DefinitionError
	if !errors.Is(err, ErrInvalidDefinition) || !errors.As(err, &de) || de.Field != field {
		t.Fatalf("error = %v, want DefinitionError field %s", err, field)
	}
}

func TestValidateDefinitionOrder(t *testing.T) {
	d := Definition{}
	steps := []struct {
		field string
		fix   func()
	}{
		{"Op", func() { d.Op = OperationRatio }},
		{"A.TableID", func() { d.A.TableID = "table" }},
		{"A.Code", func() { d.A.Code = "a" }},
		{"B", func() { d.B = &Item{} }},
		{"B.TableID", func() { d.B.TableID = "table" }},
		{"B.Code", func() { d.B.Code = "b" }},
		{"YearRule", func() { d.YearRule = YearRuleFixed }},
		{"FixedYear", func() { d.FixedYear = 2024 }},
		{"SpanYears", func() { d.SpanYears = 1 }},
		{"Scale", func() { d.Scale, d.Decimals = big.NewRat(1, 1), -1 }},
		{"Decimals", func() { d.Decimals = 0 }},
	}
	for _, step := range steps {
		checkDefinitionError(t, ValidateDefinition(d), step.field)
		step.fix()
	}
	if err := ValidateDefinition(d); err != nil {
		t.Fatal(err)
	}
}
