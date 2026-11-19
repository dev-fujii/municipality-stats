package estat

import "testing"

func TestZeroValueIsNotOK(t *testing.T) {
	if (Value{}).Status == StatusOK {
		t.Fatal("zero Value must not be StatusOK")
	}
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Value
		wantErr bool
	}{
		{name: "decimal", input: "102.55", want: Value{Raw: "102.55", Status: StatusOK}},
		{name: "zero", input: "0", want: Value{Raw: "0", Status: StatusOK}},
		{name: "negative", input: "-12.3", want: Value{Raw: "-12.3", Status: StatusOK}},
		{name: "not surveyed", input: "***", want: Value{Raw: "***", Status: StatusNotSurveyed}},
		{name: "unavailable", input: "-", want: Value{Raw: "-", Status: StatusUnavailable}},
		{name: "confidential", input: "X", want: Value{Raw: "X", Status: StatusConfidential}},
		{name: "lowercase x", input: "x", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "comma", input: "1,234", wantErr: true},
		{name: "exponent", input: "1e3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseValue(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseValue(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseValue(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseYear(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "year", input: "2023100000", want: 2023},
		{name: "not yearly", input: "2023000000", wantErr: true},
		{name: "month", input: "2023000101", wantErr: true},
		{name: "short", input: "202310000", wantErr: true},
		{name: "long", input: "20231000000", wantErr: true},
		{name: "sign", input: "-023100000", wantErr: true},
		{name: "leading minus", input: "-2023100000", wantErr: true},
		{name: "full-width digits", input: "２０２３100000", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseYear(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseYear(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseYear(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
