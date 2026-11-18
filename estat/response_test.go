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
