package estat

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// TestOneOrManyUnmarshalJSON は、配列と単一オブジェクトを並びとして読み、ほかの形をエラーにすることを確かめる。
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

// TestNoteJSON は、NOTE から記号とその意味の文を読み取れることを確かめる。
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

// TestValueJSONUnmarshalJSON は、VALUE を読み、@unit の欠け・null を許し、未知のキーや必須値の不備をエラーにすることを確かめる。
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

// TestOneOrManyValueJSON は、単一オブジェクトの VALUE も記号の文字列を保って並びとして読めることを確かめる。
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

// TestGetStatsDataResponse は、保存した応答から処理結果・続きの取得位置・注記・値を読み取れることを確かめる。
func TestGetStatsDataResponse(t *testing.T) {
	data, err := os.ReadFile("testdata/get_stats_data.json")
	if err != nil {
		t.Fatal(err)
	}

	var got getStatsDataResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}

	body := got.GetStatsData
	if body.Result.Status != 0 || body.Result.ErrorMsg != "正常に終了しました。" {
		t.Errorf("Result = %+v", body.Result)
	}
	if body.StatisticalData.ResultInf.NextKey != 5 {
		t.Errorf("NextKey = %d, want 5", body.StatisticalData.ResultInf.NextKey)
	}

	dataInf := body.StatisticalData.DataInf
	if len(dataInf.Note) != 3 {
		t.Errorf("len(Note) = %d, want 3", len(dataInf.Note))
	}
	if len(dataInf.Value) != 2 || dataInf.Value[1].Area != "10424" || dataInf.Value[1].Raw != "-" {
		t.Errorf("Value = %+v", dataInf.Value)
	}
}
