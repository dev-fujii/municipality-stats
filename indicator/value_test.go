package indicator

import (
	"testing"

	"github.com/dev-fujii/municipality-stats/estat"
)

func TestDecimalConversionAndRounding(t *testing.T) {
	tests := []struct {
		raw      string
		decimals int
		value    string
		display  string
	}{
		{"1.24", 1, "31/25", "1.2"},
		{"1.25", 1, "5/4", "1.3"},
		{"-1.24", 1, "-31/25", "-1.2"},
		{"-1.25", 1, "-5/4", "-1.3"},
		{"-0.04", 1, "-1/25", "0.0"},
		{"1.99", 1, "199/100", "2.0"},
		{"-1.5", 0, "-3/2", "-2"},
		{"1.5", 0, "3/2", "2"},
		{"-0.4", 0, "-2/5", "0"},
		{"2", 1, "2", "2.0"},
		{"0", 1, "0", "0.0"},
		{"0", 0, "0", "0"},
		{"102.55", 2, "2051/20", "102.55"},
		{"001.20", 2, "6/5", "1.20"},
		{"00010", 0, "10", "10"},
		{"-12.3", 2, "-123/10", "-12.30"},
		{"-0.0", 1, "0", "0.0"},
		{"0.00125", 4, "1/800", "0.0013"},
		{"1.234567895", 8, "246913579/200000000", "1.23456790"},
		{"2", 20, "2", "2.00000000000000000000"},
		{"999999999999999999999.95", 1, "19999999999999999999999/20", "1000000000000000000000.0"},
	}
	for _, tt := range tests {
		t.Run(tt.raw+"/"+tt.display, func(t *testing.T) {
			d := testDefinition(OperationValue)
			d.Decimals = tt.decimals
			got, err := Calculate(d, []string{"10201"}, []estat.Record{testRecord("a", "10201", 2024, tt.raw)})
			if err != nil {
				t.Fatal(err)
			}
			checkResult(t, got[0], "10201", tt.value, tt.display, &YearRange{2024, 2024}, estat.StatusOK, CalculationStatusOK)
		})
	}
}
