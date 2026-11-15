package estat

import "testing"

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
