package estat

import (
	"maps"
	"testing"
)

// TestCheckNotes は、既知の記号と意味の文だけを受け付け、載っていた記号を返すことを確かめる。
func TestCheckNotes(t *testing.T) {
	tests := []struct {
		name    string
		notes   []noteJSON
		want    map[string]bool
		wantErr bool
	}{
		{
			name: "all symbols",
			notes: []noteJSON{
				{Char: "***", Text: "調査又は集計していないもの"},
				{Char: "-", Text: "データが得られないもの"},
				{Char: "X", Text: "数値が秘匿されているもの"},
			},
			want: map[string]bool{"***": true, "-": true, "X": true},
		},
		{name: "spaces around text", notes: []noteJSON{{Char: "-", Text: "　データが得られないもの "}}, want: map[string]bool{"-": true}},
		{name: "no notes", notes: nil, want: map[string]bool{}},
		{name: "unknown symbol", notes: []noteJSON{{Char: "x", Text: "数値が秘匿されているもの"}}, wantErr: true},
		{name: "different meaning", notes: []noteJSON{{Char: "-", Text: "該当数値なし"}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkNotes(tt.notes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkNotes() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("checkNotes() = %v, want %v", got, tt.want)
			}
		})
	}
}
