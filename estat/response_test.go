package estat

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestOneOrManyUnmarshalJSON(t *testing.T) {
	type item struct {
		Raw string `json:"$"`
	}

	tests := []struct {
		name    string
		input   string
		want    oneOrMany[item]
		wantErr bool
	}{
		{name: "many", input: `[{"$":"102.55"},{"$":"98.10"}]`, want: oneOrMany[item]{{Raw: "102.55"}, {Raw: "98.10"}}},
		{name: "one", input: `{"$":"102.55"}`, want: oneOrMany[item]{{Raw: "102.55"}}},
		{name: "empty array", input: `[]`, want: oneOrMany[item]{}},
		{name: "null", input: `null`, wantErr: true},
		{name: "string", input: `"102.55"`, wantErr: true},
		{name: "number", input: `102.55`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got oneOrMany[item]
			err := json.Unmarshal([]byte(tt.input), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unmarshal(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Unmarshal(%s) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNoteJSON(t *testing.T) {
	input := `[
		{"@char":"***","$":"調査又は集計していないもの"},
		{"@char":"-","$":"データが得られないもの"}
	]`
	want := oneOrMany[noteJSON]{
		{Char: "***", Text: "調査又は集計していないもの"},
		{Char: "-", Text: "データが得られないもの"},
	}

	var got oneOrMany[noteJSON]
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestValueJSONUnmarshalJSON(t *testing.T) {
	want := valueJSON{Tab: "00001", Cat01: "H2130", Area: "10201", Time: "2023100000", Unit: "ｍ2", Raw: "102.55"}
	noUnit := want
	noUnit.Unit = ""

	tests := []struct {
		name    string
		input   string
		want    valueJSON
		wantErr bool
	}{
		{name: "all keys", input: `{"@tab":"00001","@cat01":"H2130","@area":"10201","@time":"2023100000","@unit":"ｍ2","$":"102.55"}`, want: want},
		{name: "no unit", input: `{"@tab":"00001","@cat01":"H2130","@area":"10201","@time":"2023100000","$":"102.55"}`, want: noUnit},
		{name: "null unit", input: `{"@tab":"00001","@cat01":"H2130","@area":"10201","@time":"2023100000","@unit":null,"$":"102.55"}`, want: noUnit},
		{name: "unknown key", input: `{"@tab":"00001","@cat01":"H2130","@cat02":"A","@area":"10201","@time":"2023100000","$":"102.55"}`, wantErr: true},
		{name: "missing area", input: `{"@tab":"00001","@cat01":"H2130","@time":"2023100000","$":"102.55"}`, wantErr: true},
		{name: "empty area", input: `{"@tab":"00001","@cat01":"H2130","@area":"","@time":"2023100000","$":"102.55"}`, wantErr: true},
		{name: "null area", input: `{"@tab":"00001","@cat01":"H2130","@area":null,"@time":"2023100000","$":"102.55"}`, wantErr: true},
		{name: "uppercase key", input: `{"@tab":"00001","@cat01":"H2130","@AREA":"10201","@time":"2023100000","$":"102.55"}`, wantErr: true},
		{name: "number value", input: `{"@tab":"00001","@cat01":"H2130","@area":"10201","@time":"2023100000","$":102.55}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got valueJSON
			err := json.Unmarshal([]byte(tt.input), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unmarshal(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Unmarshal(%s) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestOneOrManyValueJSON(t *testing.T) {
	input := `{"@tab":"00001","@cat01":"H2130","@area":"10201","@time":"2023100000","$":"-"}`
	want := oneOrMany[valueJSON]{{Tab: "00001", Cat01: "H2130", Area: "10201", Time: "2023100000", Raw: "-"}}

	var got oneOrMany[valueJSON]
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
