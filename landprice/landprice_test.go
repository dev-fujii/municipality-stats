package landprice_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dev-fujii/municipality-stats/landprice"
)

func int64Ptr(v int64) *int64 { return &v }

func snapshot(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func readTestdata(t testing.TB, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func memoryZIP(t testing.TB, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type closeTrackingReaderAt struct {
	*bytes.Reader
	closed bool
}

func (c *closeTrackingReaderAt) Close() error {
	c.closed = true
	return nil
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestReadGeoJSONAndReadZip(t *testing.T) {
	raw := readTestdata(t, "testdata/l02_sample.geojson")

	fromJSON, err := landprice.ReadGeoJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadGeoJSON failed: %v", err)
	}
	if len(fromJSON) != 8 {
		t.Fatalf("len=%d want=8", len(fromJSON))
	}
	wantFirst := landprice.Record{
		UseCode:   "000",
		Serial:    "001",
		Year:      2026,
		Price:     100000,
		AreaCode:  "99101",
		PlaceName: "中央一丁目",
		Point:     landprice.Point{Latitude: 36.001, Longitude: 139.001},
	}
	if fromJSON[0] != wantFirst {
		t.Fatalf("first=%+v want=%+v", fromJSON[0], wantFirst)
	}

	zipBytes := memoryZIP(t, map[string][]byte{
		"L02-26_99.shp":     []byte("ignored"),
		"KS-META-L02.xml":   []byte("<meta/>"),
		"L02-26_99.geojson": raw,
	})
	tracker := &closeTrackingReaderAt{Reader: bytes.NewReader(zipBytes)}
	fromZIP, err := landprice.ReadZip(landprice.ZIPSource{
		Name:   "L02-26_99_GML.zip",
		Reader: tracker,
		Size:   int64(len(zipBytes)),
	})
	if err != nil {
		t.Fatalf("ReadZip failed: %v", err)
	}
	if tracker.closed {
		t.Fatal("ReadZip closed caller Reader")
	}
	if !reflect.DeepEqual(fromZIP, fromJSON) {
		t.Fatalf("ReadZip=%+v ReadGeoJSON=%+v", fromZIP, fromJSON)
	}
}

func TestReadGeoJSONEmptyFeatures(t *testing.T) {
	got, err := landprice.ReadGeoJSON(strings.NewReader(`{"type":"FeatureCollection","features":[]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len=%d want=0", len(got))
	}
}

func TestReadGeoJSONErrors(t *testing.T) {
	validFeature := func(props, geom string) string {
		return `{"type":"FeatureCollection","features":[{"type":"Feature","properties":` + props + `,"geometry":` + geom + `}]}`
	}
	validGeom := `{"type":"Point","coordinates":[139.0,36.0]}`

	for _, tt := range []struct {
		name      string
		input     []byte
		wantField string
	}{
		{
			name:      "invalid UTF-8",
			input:     []byte{0xff, 0xfe},
			wantField: "JSON",
		},
		{
			name:      "trailing JSON",
			input:     []byte(`{"type":"FeatureCollection","features":[]}{}`),
			wantField: "JSON",
		},
		{
			name:      "wrong top type",
			input:     []byte(`{"type":"Feature","features":[]}`),
			wantField: "Type",
		},
		{
			name:      "null features",
			input:     []byte(`{"type":"FeatureCollection","features":null}`),
			wantField: "Features",
		},
		{
			name:      "missing L02_001",
			input:     []byte(validFeature(`{"L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_001",
		},
		{
			name:      "missing L02_002",
			input:     []byte(validFeature(`{"L02_001":"000","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_002",
		},
		{
			name:      "missing L02_005",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_006":1000,"L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_005",
		},
		{
			name:      "L02_005 string instead of integer",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":"2026","L02_006":1000,"L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_005",
		},
		{
			name:      "L02_005 float instead of integer",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026.5,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_005",
		},
		{
			name:      "L02_005 non-positive",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":0,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_005",
		},
		{
			name:      "missing L02_006",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_006",
		},
		{
			name:      "negative price",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":-1,"L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_006",
		},
		{
			name:      "price wrong type",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":"1000","L02_020":"99201","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_006",
		},
		{
			name:      "missing L02_020",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_020",
		},
		{
			name:      "invalid L02_020 length",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"9920","L02_021":"a"}`, validGeom)),
			wantField: "Features[0].Properties.L02_020",
		},
		{
			name:      "missing L02_021",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201"}`, validGeom)),
			wantField: "Features[0].Properties.L02_021",
		},
		{
			name:      "L02_021 wrong type",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":123}`, validGeom)),
			wantField: "Features[0].Properties.L02_021",
		},
		{
			name:      "null geometry",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, `null`)),
			wantField: "Features[0].Geometry",
		},
		{
			name:      "non-Point geometry",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, `{"type":"LineString","coordinates":[[139,36],[140,37]]}`)),
			wantField: "Features[0].Geometry.Type",
		},
		{
			name:      "empty coordinates",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, `{"type":"Point","coordinates":[]}`)),
			wantField: "Features[0].Geometry.Coordinates",
		},
		{
			name:      "string coordinate",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, `{"type":"Point","coordinates":["139.0",36.0]}`)),
			wantField: "Features[0].Geometry.Coordinates",
		},
		{
			name:      "coordinate out of range",
			input:     []byte(validFeature(`{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":"a"}`, `{"type":"Point","coordinates":[181.0,36.0]}`)),
			wantField: "Features[0].Geometry.Coordinates",
		},
		{
			name: "mixed years in single file",
			input: []byte(`{"type":"FeatureCollection","features":[
				{"type":"Feature","properties":{"L02_001":"000","L02_002":"001","L02_005":2026,"L02_006":1000,"L02_020":"99201","L02_021":"a"},"geometry":{"type":"Point","coordinates":[139.0,36.0]}},
				{"type":"Feature","properties":{"L02_001":"000","L02_002":"002","L02_005":2025,"L02_006":2000,"L02_020":"99201","L02_021":"b"},"geometry":{"type":"Point","coordinates":[139.1,36.1]}}
			]}`),
			wantField: "Features[1].Properties.L02_005",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := landprice.ReadGeoJSON(bytes.NewReader(tt.input))
			if err == nil {
				t.Fatalf("expected error, got %+v", got)
			}
			if got != nil {
				t.Fatalf("partial result=%+v want=nil", got)
			}
			if !errors.Is(err, landprice.ErrInvalidFormat) {
				t.Fatalf("err=%v want ErrInvalidFormat", err)
			}
			var re *landprice.ReadError
			if !errors.As(err, &re) {
				t.Fatalf("err=%v want *ReadError", err)
			}
			if re.Field != tt.wantField {
				t.Fatalf("Field=%q want=%q (err=%v)", re.Field, tt.wantField, err)
			}
		})
	}
}

func TestReadGeoJSONArgumentAndIOErrors(t *testing.T) {
	var typedNil *bytes.Reader
	for _, tt := range []struct {
		name string
		r    io.Reader
		want error
	}{
		{"nil reader", nil, landprice.ErrInvalidArgument},
		{"typed nil reader", typedNil, landprice.ErrInvalidArgument},
		{"io error", errReader{err: io.ErrUnexpectedEOF}, io.ErrUnexpectedEOF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := landprice.ReadGeoJSON(tt.r)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("got=%+v err=%v want=%v", got, err, tt.want)
			}
		})
	}
}

func TestReadZipErrors(t *testing.T) {
	sample := readTestdata(t, "testdata/l02_sample.geojson")
	var typedNil *bytes.Reader

	noGeoJSON := memoryZIP(t, map[string][]byte{"L02-26_99.shp": []byte("x")})
	twoGeoJSON := memoryZIP(t, map[string][]byte{
		"a.geojson": sample,
		"b.geojson": sample,
	})
	var storeBuf bytes.Buffer
	sw := zip.NewWriter(&storeBuf)
	sf, err := sw.CreateHeader(&zip.FileHeader{Name: "L02-26_99.geojson", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf.Write(sample); err != nil {
		t.Fatal(err)
	}
	if err := sw.Close(); err != nil {
		t.Fatal(err)
	}
	corruptCRC := storeBuf.Bytes()
	idx := bytes.Index(corruptCRC, []byte("99101"))
	if idx < 0 {
		t.Fatal("payload marker not found in stored zip")
	}
	corruptCRC[idx] ^= 0xff

	for _, tt := range []struct {
		name      string
		src       landprice.ZIPSource
		wantIs    error
		wantSrc   string
		wantField string
	}{
		{
			name:   "nil reader",
			src:    landprice.ZIPSource{Name: "a.zip", Reader: nil, Size: 10},
			wantIs: landprice.ErrInvalidArgument,
		},
		{
			name:   "typed nil reader",
			src:    landprice.ZIPSource{Name: "a.zip", Reader: typedNil, Size: 10},
			wantIs: landprice.ErrInvalidArgument,
		},
		{
			name:   "non-positive size",
			src:    landprice.ZIPSource{Name: "a.zip", Reader: bytes.NewReader(noGeoJSON), Size: 0},
			wantIs: landprice.ErrInvalidArgument,
		},
		{
			name:      "not a zip",
			src:       landprice.ZIPSource{Name: "bad.zip", Reader: bytes.NewReader([]byte("not-zip")), Size: 7},
			wantIs:    zip.ErrFormat,
			wantSrc:   "bad.zip",
			wantField: "File",
		},
		{
			name:      "missing geojson",
			src:       landprice.ZIPSource{Name: "empty.zip", Reader: bytes.NewReader(noGeoJSON), Size: int64(len(noGeoJSON))},
			wantIs:    landprice.ErrInvalidFormat,
			wantSrc:   "empty.zip",
			wantField: "File",
		},
		{
			name:      "multiple geojson",
			src:       landprice.ZIPSource{Name: "two.zip", Reader: bytes.NewReader(twoGeoJSON), Size: int64(len(twoGeoJSON))},
			wantIs:    landprice.ErrInvalidFormat,
			wantSrc:   "two.zip",
			wantField: "File",
		},
		{
			name:      "corrupt zip checksum",
			src:       landprice.ZIPSource{Name: "corrupt.zip", Reader: bytes.NewReader(corruptCRC), Size: int64(len(corruptCRC))},
			wantIs:    zip.ErrChecksum,
			wantSrc:   "corrupt.zip",
			wantField: "File",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := landprice.ReadZip(tt.src)
			if err == nil || got != nil {
				t.Fatalf("got=%+v err=%v want error", got, err)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("err=%v wantIs=%v", err, tt.wantIs)
			}
			if tt.wantField != "" {
				var re *landprice.ReadError
				if !errors.As(err, &re) {
					t.Fatalf("err=%v want *ReadError", err)
				}
				if re.Source != tt.wantSrc || re.Field != tt.wantField {
					t.Fatalf("ReadError={Source:%q Field:%q} want={Source:%q Field:%q}", re.Source, re.Field, tt.wantSrc, tt.wantField)
				}
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	records, err := landprice.ReadGeoJSON(bytes.NewReader(readTestdata(t, "testdata/l02_sample.geojson")))
	if err != nil {
		t.Fatal(err)
	}
	rollup := map[string]string{
		"99101": "99100",
		"99102": "99100",
	}
	areas := []string{"99203", "99201", "99100", "99202"}

	for _, tt := range []struct {
		name    string
		records []landprice.Record
		useCode string
		rollup  map[string]string
		areas   []string
		want    []landprice.Summary
	}{
		{
			name:    "residential with rollup, 0/1/2(round-up)/3 points, ignores commercial and forest PP900",
			records: records,
			useCode: landprice.UseResidential,
			rollup:  rollup,
			areas:   areas,
			want: []landprice.Summary{
				{AreaCode: "99100", State: landprice.SummaryOK, Count: 2, Median: int64Ptr(100001), Min: int64Ptr(100000), Max: int64Ptr(100001), Year: 2026},
				{AreaCode: "99201", State: landprice.SummaryOK, Count: 1, Median: int64Ptr(50000), Min: int64Ptr(50000), Max: int64Ptr(50000), Year: 2026},
				{AreaCode: "99202", State: landprice.SummaryOK, Count: 3, Median: int64Ptr(60000), Min: int64Ptr(40000), Max: int64Ptr(80000), Year: 2026},
				{AreaCode: "99203", State: landprice.SummaryNoPoint},
			},
		},
		{
			name:    "commercial only",
			records: records,
			useCode: landprice.UseCommercial,
			rollup:  rollup,
			areas:   []string{"99202", "99201"},
			want: []landprice.Summary{
				{AreaCode: "99201", State: landprice.SummaryNoPoint},
				{AreaCode: "99202", State: landprice.SummaryOK, Count: 1, Median: int64Ptr(250000), Min: int64Ptr(250000), Max: int64Ptr(250000), Year: 2026},
			},
		},
		{
			name:    "forest PP900 succeeds when included in areaCodes",
			records: records,
			useCode: landprice.UseForest,
			rollup:  nil,
			areas:   []string{"99900", "99201"},
			want: []landprice.Summary{
				{AreaCode: "99201", State: landprice.SummaryNoPoint},
				{AreaCode: "99900", State: landprice.SummaryOK, Count: 1, Median: int64Ptr(32000), Min: int64Ptr(32000), Max: int64Ptr(32000), Year: 2026},
			},
		},
		{
			name: "even median exact average and large int64 without overflow",
			records: []landprice.Record{
				{UseCode: "000", Year: 2026, Price: 10000, AreaCode: "99201"},
				{UseCode: "000", Year: 2026, Price: 10002, AreaCode: "99201"},
				{UseCode: "000", Year: 2026, Price: math.MaxInt64 - 1, AreaCode: "99202"},
				{UseCode: "000", Year: 2026, Price: math.MaxInt64, AreaCode: "99202"},
			},
			useCode: "000",
			rollup:  nil,
			areas:   []string{"99202", "99201"},
			want: []landprice.Summary{
				{AreaCode: "99201", State: landprice.SummaryOK, Count: 2, Median: int64Ptr(10001), Min: int64Ptr(10000), Max: int64Ptr(10002), Year: 2026},
				{AreaCode: "99202", State: landprice.SummaryOK, Count: 2, Median: int64Ptr(math.MaxInt64), Min: int64Ptr(math.MaxInt64 - 1), Max: int64Ptr(math.MaxInt64), Year: 2026},
			},
		},
		{
			name:    "empty records returns all areas as no_point",
			records: nil,
			useCode: "000",
			rollup:  nil,
			areas:   []string{"99202", "99201"},
			want: []landprice.Summary{
				{AreaCode: "99201", State: landprice.SummaryNoPoint},
				{AreaCode: "99202", State: landprice.SummaryNoPoint},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			beforeRecs := snapshot(t, tt.records)
			beforeRollup := snapshot(t, tt.rollup)
			beforeAreas := snapshot(t, tt.areas)

			got, err := landprice.Summarize(tt.records, tt.useCode, tt.rollup, tt.areas)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got=%s want=%s", snapshot(t, got), snapshot(t, tt.want))
			}

			// 入力順を逆にしても同じ結果になることを確認する。
			rev := slices.Clone(tt.records)
			slices.Reverse(rev)
			gotRev, err := landprice.Summarize(rev, tt.useCode, tt.rollup, tt.areas)
			if err != nil {
				t.Fatalf("reverse unexpected error: %v", err)
			}
			if !reflect.DeepEqual(gotRev, tt.want) {
				t.Fatalf("reverse got=%s want=%s", snapshot(t, gotRev), snapshot(t, tt.want))
			}

			if !bytes.Equal(beforeRecs, snapshot(t, tt.records)) ||
				!bytes.Equal(beforeRollup, snapshot(t, tt.rollup)) ||
				!bytes.Equal(beforeAreas, snapshot(t, tt.areas)) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestSummarizeErrors(t *testing.T) {
	records, err := landprice.ReadGeoJSON(bytes.NewReader(readTestdata(t, "testdata/l02_sample.geojson")))
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name    string
		records []landprice.Record
		useCode string
		rollup  map[string]string
		areas   []string
		wantIs  error
		wantKey string
	}{
		{
			name:    "forest PP900 fails when forest selected and PP900 not in areaCodes",
			records: records,
			useCode: landprice.UseForest,
			rollup:  nil,
			areas:   []string{"99201", "99202"},
			wantIs:  landprice.ErrUnknownArea,
			wantKey: "99900",
		},
		{
			name:    "unrolled ward code fails when rollup omitted",
			records: records,
			useCode: landprice.UseResidential,
			rollup:  nil,
			areas:   []string{"99100", "99201", "99202"},
			wantIs:  landprice.ErrUnknownArea,
			wantKey: "99101",
		},
		{
			name: "mixed years among selected useCode",
			records: []landprice.Record{
				{UseCode: "000", Year: 2026, Price: 10000, AreaCode: "99201"},
				{UseCode: "000", Year: 2025, Price: 20000, AreaCode: "99202"},
			},
			useCode: "000",
			areas:   []string{"99201", "99202"},
			wantIs:  landprice.ErrInvalidFormat,
			wantKey: "99202",
		},
		{
			name: "negative price in selected record",
			records: []landprice.Record{
				{UseCode: "000", Year: 2026, Price: -1, AreaCode: "99201"},
			},
			useCode: "000",
			areas:   []string{"99201"},
			wantIs:  landprice.ErrInvalidFormat,
			wantKey: "99201",
		},
		{
			name:    "empty useCode",
			records: nil,
			useCode: "",
			areas:   []string{"99201"},
			wantIs:  landprice.ErrInvalidArgument,
		},
		{
			name:    "empty areaCodes",
			records: nil,
			useCode: "000",
			areas:   nil,
			wantIs:  landprice.ErrInvalidArgument,
		},
		{
			name:    "duplicate areaCodes",
			records: nil,
			useCode: "000",
			areas:   []string{"99201", "99201"},
			wantIs:  landprice.ErrInvalidArgument,
		},
		{
			name:    "chained rollup",
			records: nil,
			useCode: "000",
			rollup:  map[string]string{"99101": "99102", "99102": "99100"},
			areas:   []string{"99100"},
			wantIs:  landprice.ErrInvalidArgument,
		},
		{
			name:    "areaCodes contains rolled-up ward",
			records: nil,
			useCode: "000",
			rollup:  map[string]string{"99101": "99100"},
			areas:   []string{"99100", "99101"},
			wantIs:  landprice.ErrInvalidArgument,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := landprice.Summarize(tt.records, tt.useCode, tt.rollup, tt.areas)
			if err == nil || got != nil {
				t.Fatalf("got=%+v err=%v want error", got, err)
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("err=%v wantIs=%v", err, tt.wantIs)
			}
			if tt.wantKey != "" {
				var re *landprice.ReadError
				if !errors.As(err, &re) {
					t.Fatalf("err=%v want *ReadError", err)
				}
				if re.Key != tt.wantKey || !strings.Contains(err.Error(), tt.wantKey) {
					t.Fatalf("Key=%q err=%q want key %q", re.Key, err.Error(), tt.wantKey)
				}
			}
		})
	}
}
