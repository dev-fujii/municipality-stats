package estat

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
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

// TestRecordBuilderAdd は、1つの応答の値と記号を Record に変え、不正な応答では builder を変えずにエラーにすることを確かめる。
func TestRecordBuilderAdd(t *testing.T) {
	base := valueJSON{Tab: "00001", Cat01: "H2130", Area: "10201", Time: "2023100000", Unit: "ｍ2", Raw: "102.55"}
	wantBase := Record{TableID: "0000020208", Code: "H2130", Area: "10201", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "102.55", Status: StatusOK}}
	changed := func(change func(*valueJSON)) valueJSON {
		v := base
		change(&v)
		return v
	}
	notes := oneOrMany[noteJSON]{
		{Char: "***", Text: "調査又は集計していないもの"},
		{Char: "-", Text: "データが得られないもの"},
		{Char: "X", Text: "数値が秘匿されているもの"},
	}

	tests := []struct {
		name    string
		data    dataInfJSON
		want    []Record
		wantErr string
	}{
		{
			name: "number and all symbols",
			data: dataInfJSON{Note: notes, Value: oneOrMany[valueJSON]{
				base,
				changed(func(v *valueJSON) { v.Area, v.Raw = "10424", "***" }),
				changed(func(v *valueJSON) { v.Area, v.Raw = "10425", "-" }),
				changed(func(v *valueJSON) { v.Area, v.Raw = "10426", "X" }),
			}},
			want: []Record{
				wantBase,
				{TableID: "0000020208", Code: "H2130", Area: "10424", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "***", Status: StatusNotSurveyed}},
				{TableID: "0000020208", Code: "H2130", Area: "10425", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "-", Status: StatusUnavailable}},
				{TableID: "0000020208", Code: "H2130", Area: "10426", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "X", Status: StatusConfidential}},
			},
		},
		{
			name: "no unit",
			data: dataInfJSON{Value: oneOrMany[valueJSON]{changed(func(v *valueJSON) { v.Unit = "" })}},
			want: []Record{{TableID: "0000020208", Code: "H2130", Area: "10201", Year: 2023, Value: Value{Raw: "102.55", Status: StatusOK}}},
		},
		{
			name: "strings are preserved",
			data: dataInfJSON{Value: oneOrMany[valueJSON]{changed(func(v *valueJSON) { v.Cat01, v.Area, v.Unit = "#A01202", "1", " 人 " })}},
			want: []Record{{TableID: "0000020208", Code: "#A01202", Area: "1", Year: 2023, Unit: " 人 ", Value: Value{Raw: "102.55", Status: StatusOK}}},
		},
		{
			name:    "symbol absent from notes",
			data:    dataInfJSON{Note: notes[:2], Value: oneOrMany[valueJSON]{base, changed(func(v *valueJSON) { v.Area, v.Raw = "10424", "X" })}},
			wantErr: `VALUE[1]: symbol "X" is not in NOTE`,
		},
		{
			name:    "symbol without notes",
			data:    dataInfJSON{Value: oneOrMany[valueJSON]{base, changed(func(v *valueJSON) { v.Area, v.Raw = "10424", "-" })}},
			wantErr: `VALUE[1]: symbol "-" is not in NOTE`,
		},
		{name: "number without notes", data: dataInfJSON{Value: oneOrMany[valueJSON]{base}}, want: []Record{wantBase}},
		{
			name:    "unknown note symbol",
			data:    dataInfJSON{Note: oneOrMany[noteJSON]{{Char: "x", Text: "数値が秘匿されているもの"}}, Value: oneOrMany[valueJSON]{base}},
			wantErr: `unknown NOTE symbol "x"`,
		},
		{
			name:    "different note meaning",
			data:    dataInfJSON{Note: oneOrMany[noteJSON]{{Char: "-", Text: "該当数値なし"}}, Value: oneOrMany[valueJSON]{base}},
			wantErr: `NOTE text for "-"`,
		},
		{
			name:    "nonannual time",
			data:    dataInfJSON{Value: oneOrMany[valueJSON]{base, changed(func(v *valueJSON) { v.Time = "2023000000" })}},
			wantErr: `VALUE[1]: @time "2023000000"`,
		},
		{
			name:    "invalid value",
			data:    dataInfJSON{Value: oneOrMany[valueJSON]{base, changed(func(v *valueJSON) { v.Raw = "1,234" })}},
			wantErr: `VALUE[1]: value "1,234"`,
		},
		{
			name:    "different tab",
			data:    dataInfJSON{Value: oneOrMany[valueJSON]{base, changed(func(v *valueJSON) { v.Tab = "00002" })}},
			wantErr: `VALUE[1]: @tab "00002"`,
		},
		{
			name:    "duplicate key with different value and unit",
			data:    dataInfJSON{Value: oneOrMany[valueJSON]{base, changed(func(v *valueJSON) { v.Raw, v.Unit = "1", "人" })}},
			wantErr: `VALUE[1]: duplicate record for code "H2130", area "10201", @time "2023100000"`,
		},
		{
			name: "different code area or year",
			data: dataInfJSON{Value: oneOrMany[valueJSON]{
				base,
				changed(func(v *valueJSON) { v.Cat01 = "H2131" }),
				changed(func(v *valueJSON) { v.Area = "10424" }),
				changed(func(v *valueJSON) { v.Time = "2024100000" }),
			}},
			want: []Record{
				wantBase,
				{TableID: "0000020208", Code: "H2131", Area: "10201", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "102.55", Status: StatusOK}},
				{TableID: "0000020208", Code: "H2130", Area: "10424", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "102.55", Status: StatusOK}},
				{TableID: "0000020208", Code: "H2130", Area: "10201", Year: 2024, Unit: "ｍ2", Value: Value{Raw: "102.55", Status: StatusOK}},
			},
		},
		{name: "empty values", data: dataInfJSON{}},
		{name: "invalid note without values", data: dataInfJSON{Note: oneOrMany[noteJSON]{{Char: "?", Text: "不明"}}}, wantErr: `unknown NOTE symbol "?"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newRecordBuilder("0000020208")
			before := slices.Clone(b.result())
			beforeTab, beforeSeen := b.tab, maps.Clone(b.seen)
			err := b.add(tt.data)
			if (err != nil) != (tt.wantErr != "") {
				t.Fatalf("add() error = %v, wantErr %q", err, tt.wantErr)
			}
			if err != nil {
				if !strings.HasPrefix(err.Error(), "e-Stat: ") || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("add() error = %v, want prefix e-Stat: and text %q", err, tt.wantErr)
				}
				if !slices.Equal(b.result(), before) || b.tab != beforeTab || !maps.Equal(b.seen, beforeSeen) {
					t.Fatalf("add() changed builder after error: %+v", b)
				}
			}
			if !slices.Equal(b.result(), tt.want) {
				t.Errorf("result() = %+v, want %+v", b.result(), tt.want)
			}
		})
	}
}

// TestRecordBuilderResponses は、@tab と重複の検査が応答をまたいで効き、NOTE は応答ごとに照らすことを確かめる。
func TestRecordBuilderResponses(t *testing.T) {
	first := valueJSON{Tab: "00001", Cat01: "H2130", Area: "10201", Time: "2023100000", Unit: "ｍ2", Raw: "102.55"}
	second := first
	second.Area, second.Raw = "10424", "98.10"
	third := first
	third.Area, third.Raw = "10425", "100"
	otherTab := third
	otherTab.Tab = "00002"
	invalidTime := third
	invalidTime.Time = "2023000000"
	invalidValue := third
	invalidValue.Raw = "1,234"
	symbol := third
	symbol.Raw = "-"
	notes := oneOrMany[noteJSON]{{Char: "-", Text: "データが得られないもの"}}
	wantFirst := Record{TableID: "0000020208", Code: "H2130", Area: "10201", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "102.55", Status: StatusOK}}
	wantSecond := Record{TableID: "0000020208", Code: "H2130", Area: "10424", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "98.10", Status: StatusOK}}
	wantThird := Record{TableID: "0000020208", Code: "H2130", Area: "10425", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "100", Status: StatusOK}}

	tests := []struct {
		name    string
		data    dataInfJSON
		want    []Record
		wantErr string
	}{
		{name: "ordered records", data: dataInfJSON{Value: oneOrMany[valueJSON]{second, third}}, want: []Record{wantFirst, wantSecond, wantThird}},
		{name: "empty response", data: dataInfJSON{}, want: []Record{wantFirst}},
		{name: "different tab", data: dataInfJSON{Value: oneOrMany[valueJSON]{second, otherTab}}, wantErr: `VALUE[1]: @tab "00002"`},
		{name: "duplicate key", data: dataInfJSON{Value: oneOrMany[valueJSON]{second, first}}, wantErr: `VALUE[1]: duplicate record`},
		{name: "invalid time", data: dataInfJSON{Value: oneOrMany[valueJSON]{second, invalidTime}}, wantErr: `VALUE[1]: @time "2023000000"`},
		{name: "invalid value", data: dataInfJSON{Value: oneOrMany[valueJSON]{second, invalidValue}}, wantErr: `VALUE[1]: value "1,234"`},
		{name: "notes belong to each response", data: dataInfJSON{Value: oneOrMany[valueJSON]{second, symbol}}, wantErr: `VALUE[1]: symbol "-" is not in NOTE`},
		{name: "unknown note symbol", data: dataInfJSON{Note: oneOrMany[noteJSON]{{Char: "?", Text: "不明"}}, Value: oneOrMany[valueJSON]{second}}, wantErr: `unknown NOTE symbol "?"`},
		{name: "different note meaning", data: dataInfJSON{Note: oneOrMany[noteJSON]{{Char: "-", Text: "該当数値なし"}}, Value: oneOrMany[valueJSON]{second}}, wantErr: `NOTE text for "-"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newRecordBuilder("0000020208")
			if err := b.add(dataInfJSON{Note: notes, Value: oneOrMany[valueJSON]{first}}); err != nil {
				t.Fatalf("first add() error = %v", err)
			}
			before := slices.Clone(b.result())
			beforeTab, beforeSeen := b.tab, maps.Clone(b.seen)
			err := b.add(tt.data)
			if (err != nil) != (tt.wantErr != "") {
				t.Fatalf("second add() error = %v, wantErr %q", err, tt.wantErr)
			}
			if err != nil {
				if !strings.HasPrefix(err.Error(), "e-Stat: ") || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("second add() error = %v, want prefix e-Stat: and text %q", err, tt.wantErr)
				}
				if !slices.Equal(b.result(), before) || b.tab != beforeTab || !maps.Equal(b.seen, beforeSeen) {
					t.Fatalf("second add() changed builder after error: %+v", b)
				}
				if err := b.add(dataInfJSON{Value: oneOrMany[valueJSON]{second, third}}); err != nil {
					t.Fatalf("corrected add() error = %v", err)
				}
				want := []Record{wantFirst, wantSecond, wantThird}
				if !slices.Equal(b.result(), want) {
					t.Errorf("corrected result() = %+v, want %+v", b.result(), want)
				}
				return
			}
			if !slices.Equal(b.result(), tt.want) {
				t.Errorf("result() = %+v, want %+v", b.result(), tt.want)
			}
		})
	}
}

// TestRecordBuilderSavedResponse は、保存した実際の応答から Record を作れることを確かめる。
func TestRecordBuilderSavedResponse(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		tableID string
		want    []Record
	}{
		{
			name:    "get stats data",
			path:    "testdata/get_stats_data.json",
			tableID: "0000020208",
			want: []Record{
				{TableID: "0000020208", Code: "H2130", Area: "10201", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "102.55", Status: StatusOK}},
				{TableID: "0000020208", Code: "H2130", Area: "10424", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "-", Status: StatusUnavailable}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			var response getStatsDataResponse
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatalf("Unmarshal error = %v", err)
			}
			b := newRecordBuilder(tt.tableID)
			if err := b.add(response.GetStatsData.StatisticalData.DataInf); err != nil {
				t.Fatalf("add() error = %v", err)
			}
			if !slices.Equal(b.result(), tt.want) {
				t.Errorf("result() = %+v, want %+v", b.result(), tt.want)
			}
		})
	}
}
