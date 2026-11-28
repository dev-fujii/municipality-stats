package rail_test

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

	"github.com/dev-fujii/municipality-stats/rail"
)

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
	raw := readTestdata(t, "testdata/n02_station_sample.geojson")

	fromJSON, err := rail.ReadGeoJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadGeoJSON failed: %v", err)
	}
	if len(fromJSON) != 6 {
		t.Fatalf("len=%d want=6", len(fromJSON))
	}
	wantFirst := rail.Record{
		InstitutionType: "1",
		LineName:        "東北新幹線",
		Operator:        "東日本旅客鉄道",
		StationName:     "盛岡",
		StationCode:     "000792_1",
		GroupCode:       "000792",
		Coordinates: []rail.Point{
			{Latitude: 39.701, Longitude: 141.135},
			{Latitude: 39.703, Longitude: 141.137},
		},
	}
	if !reflect.DeepEqual(fromJSON[0], wantFirst) {
		t.Fatalf("first=%+v want=%+v", fromJSON[0], wantFirst)
	}

	for _, tt := range []struct {
		name  string
		files map[string][]byte
	}{
		{
			name: "standard N02-25 layout with Shift-JIS and RailroadSection ignored",
			files: map[string][]byte{
				"N02-25_GML/Shift-JIS/N02-25_Station.geojson":     []byte("ignored"),
				"N02-25_GML/UTF-8/N02-25_RailroadSection.geojson": []byte("ignored"),
				"N02-25_GML/UTF-8/N02-25_Station.geojson":         raw,
				"N02-25_GML/KS-META-N02-25.xml":                   []byte("<meta/>"),
			},
		},
		{
			name: "different edition directly under UTF-8",
			files: map[string][]byte{
				"UTF-8/N02-26_Station.geojson": raw,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			zipBytes := memoryZIP(t, tt.files)
			tracker := &closeTrackingReaderAt{Reader: bytes.NewReader(zipBytes)}
			fromZIP, err := rail.ReadZip(rail.ZIPSource{
				Name:   "N02_GML.zip",
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
				t.Fatalf("ReadZip=%+v want=%+v", fromZIP, fromJSON)
			}
		})
	}
}

func TestReadZipErrors(t *testing.T) {
	sample := readTestdata(t, "testdata/n02_station_sample.geojson")
	var typedNil *bytes.Reader

	noUTF8 := memoryZIP(t, map[string][]byte{
		"N02-25_Station.geojson":                      sample,
		"N02-25_GML/Shift-JIS/N02-25_Station.geojson": sample,
	})
	onlySection := memoryZIP(t, map[string][]byte{
		"N02-25_GML/UTF-8/N02-25_RailroadSection.geojson": sample,
	})
	twoStations := memoryZIP(t, map[string][]byte{
		"N02-25_GML/UTF-8/N02-25_Station.geojson": sample,
		"N02-26_GML/UTF-8/N02-26_Station.geojson": sample,
	})

	for _, tt := range []struct {
		name      string
		src       rail.ZIPSource
		wantIs    error
		wantSrc   string
		wantField string
	}{
		{
			name:   "nil reader",
			src:    rail.ZIPSource{Name: "a.zip", Reader: nil, Size: 10},
			wantIs: rail.ErrInvalidArgument,
		},
		{
			name:   "typed nil reader",
			src:    rail.ZIPSource{Name: "a.zip", Reader: typedNil, Size: 10},
			wantIs: rail.ErrInvalidArgument,
		},
		{
			name:   "non-positive size",
			src:    rail.ZIPSource{Name: "a.zip", Reader: bytes.NewReader(noUTF8), Size: 0},
			wantIs: rail.ErrInvalidArgument,
		},
		{
			name:      "not a zip",
			src:       rail.ZIPSource{Name: "bad.zip", Reader: bytes.NewReader([]byte("not-zip")), Size: 7},
			wantIs:    zip.ErrFormat,
			wantSrc:   "bad.zip",
			wantField: "File",
		},
		{
			name:      "Station geojson outside UTF-8 directory",
			src:       rail.ZIPSource{Name: "noutf8.zip", Reader: bytes.NewReader(noUTF8), Size: int64(len(noUTF8))},
			wantIs:    rail.ErrInvalidFormat,
			wantSrc:   "noutf8.zip",
			wantField: "File",
		},
		{
			name:      "only RailroadSection under UTF-8",
			src:       rail.ZIPSource{Name: "section.zip", Reader: bytes.NewReader(onlySection), Size: int64(len(onlySection))},
			wantIs:    rail.ErrInvalidFormat,
			wantSrc:   "section.zip",
			wantField: "File",
		},
		{
			name:      "multiple Station geojson under UTF-8",
			src:       rail.ZIPSource{Name: "two.zip", Reader: bytes.NewReader(twoStations), Size: int64(len(twoStations))},
			wantIs:    rail.ErrInvalidFormat,
			wantSrc:   "two.zip",
			wantField: "File",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rail.ReadZip(tt.src)
			if err == nil || got != nil {
				t.Fatalf("got=%+v err=%v want error", got, err)
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("err=%v wantIs=%v", err, tt.wantIs)
			}
			if tt.wantField != "" {
				var re *rail.ReadError
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

func TestReadGeoJSONErrors(t *testing.T) {
	validFeature := func(props, geom string) string {
		return `{"type":"FeatureCollection","features":[{"type":"Feature","properties":` + props + `,"geometry":` + geom + `}]}`
	}
	validProps := `{"N02_002":"1","N02_003":"東北新幹線","N02_004":"東日本旅客鉄道","N02_005":"盛岡","N02_005c":"000792_1","N02_005g":"000792"}`
	validGeom := `{"type":"LineString","coordinates":[[141.135,39.701],[141.137,39.703]]}`

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
			name:      "missing N02_002",
			input:     []byte(validFeature(`{"N02_003":"a","N02_004":"b","N02_005":"c","N02_005c":"d","N02_005g":"g"}`, validGeom)),
			wantField: "Features[0].Properties.N02_002",
		},
		{
			name:      "missing N02_003",
			input:     []byte(validFeature(`{"N02_002":"1","N02_004":"b","N02_005":"c","N02_005c":"d","N02_005g":"g"}`, validGeom)),
			wantField: "Features[0].Properties.N02_003",
		},
		{
			name:      "missing N02_004",
			input:     []byte(validFeature(`{"N02_002":"1","N02_003":"a","N02_005":"c","N02_005c":"d","N02_005g":"g"}`, validGeom)),
			wantField: "Features[0].Properties.N02_004",
		},
		{
			name:      "missing N02_005",
			input:     []byte(validFeature(`{"N02_002":"1","N02_003":"a","N02_004":"b","N02_005c":"d","N02_005g":"g"}`, validGeom)),
			wantField: "Features[0].Properties.N02_005",
		},
		{
			name:      "missing N02_005c",
			input:     []byte(validFeature(`{"N02_002":"1","N02_003":"a","N02_004":"b","N02_005":"c","N02_005g":"g"}`, validGeom)),
			wantField: "Features[0].Properties.N02_005c",
		},
		{
			name:      "missing N02_005g",
			input:     []byte(validFeature(`{"N02_002":"1","N02_003":"a","N02_004":"b","N02_005":"c","N02_005c":"d"}`, validGeom)),
			wantField: "Features[0].Properties.N02_005g",
		},
		{
			name:      "wrong property type",
			input:     []byte(validFeature(`{"N02_002":1,"N02_003":"a","N02_004":"b","N02_005":"c","N02_005c":"d","N02_005g":"g"}`, validGeom)),
			wantField: "Features[0].Properties.N02_002",
		},
		{
			name:      "Point instead of LineString",
			input:     []byte(validFeature(validProps, `{"type":"Point","coordinates":[141.135,39.701]}`)),
			wantField: "Features[0].Geometry.Type",
		},
		{
			name:      "single coordinate LineString",
			input:     []byte(validFeature(validProps, `{"type":"LineString","coordinates":[[141.135,39.701]]}`)),
			wantField: "Features[0].Geometry.Coordinates",
		},
		{
			name:      "string coordinate",
			input:     []byte(validFeature(validProps, `{"type":"LineString","coordinates":[["141.135",39.701],[141.137,39.703]]}`)),
			wantField: "Features[0].Geometry.Coordinates",
		},
		{
			name:      "out of range coordinate",
			input:     []byte(validFeature(validProps, `{"type":"LineString","coordinates":[[141.135,91.0],[141.137,39.703]]}`)),
			wantField: "Features[0].Geometry.Coordinates",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rail.ReadGeoJSON(bytes.NewReader(tt.input))
			if err == nil || got != nil {
				t.Fatalf("got=%+v err=%v want error", got, err)
			}
			if !errors.Is(err, rail.ErrInvalidFormat) {
				t.Fatalf("err=%v want ErrInvalidFormat", err)
			}
			var re *rail.ReadError
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
		{"nil reader", nil, rail.ErrInvalidArgument},
		{"typed nil reader", typedNil, rail.ErrInvalidArgument},
		{"io error", errReader{err: io.ErrUnexpectedEOF}, io.ErrUnexpectedEOF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rail.ReadGeoJSON(tt.r)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("got=%+v err=%v want=%v", got, err, tt.want)
			}
		})
	}
}

func TestMidpoint(t *testing.T) {
	for _, tt := range []struct {
		name    string
		coords  []rail.Point
		want    rail.Point
		wantErr bool
	}{
		{
			name: "two points",
			coords: []rail.Point{
				{Latitude: 36.0, Longitude: 139.0},
				{Latitude: 36.0, Longitude: 141.0},
			},
			want: rail.Point{Latitude: 36.0, Longitude: 140.0},
		},
		{
			name: "L-shape three points",
			coords: []rail.Point{
				{Latitude: 0, Longitude: 0},
				{Latitude: 0, Longitude: 3},
				{Latitude: 4, Longitude: 3},
			},
			want: rail.Point{Latitude: 0.5, Longitude: 3},
		},
		{
			name: "degenerate zero length",
			coords: []rail.Point{
				{Latitude: 35.5, Longitude: 139.5},
				{Latitude: 35.5, Longitude: 139.5},
			},
			want: rail.Point{Latitude: 35.5, Longitude: 139.5},
		},
		{
			name:    "too few points",
			coords:  []rail.Point{{Latitude: 35.0, Longitude: 139.0}},
			wantErr: true,
		},
		{
			name: "out of range point",
			coords: []rail.Point{
				{Latitude: 95.0, Longitude: 139.0},
				{Latitude: 35.0, Longitude: 139.0},
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rail.Midpoint(tt.coords)
			if tt.wantErr {
				if !errors.Is(err, rail.ErrInvalidArgument) {
					t.Fatalf("got=%+v err=%v want ErrInvalidArgument", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(got.Latitude-tt.want.Latitude) > 1e-12 || math.Abs(got.Longitude-tt.want.Longitude) > 1e-12 {
				t.Fatalf("got=%+v want=%+v", got, tt.want)
			}
		})
	}
}

func TestSelect(t *testing.T) {
	sampleRecords, err := rail.ReadGeoJSON(bytes.NewReader(readTestdata(t, "testdata/n02_station_sample.geojson")))
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name    string
		records []rail.Record
		types   []string
		extras  []rail.ExtraStation
		want    []rail.Station
	}{
		{
			name:  "shinkansen only including duplicate-line station Joetsumyoko",
			types: []string{rail.InstitutionShinkansen},
			want: []rail.Station{
				{
					GroupCode: "000792",
					Name:      "盛岡",
					Lines:     []string{"東北新幹線"},
					Positions: []rail.Point{{Latitude: 39.702, Longitude: 141.136}},
				},
				{
					GroupCode: "001200",
					Name:      "上越妙高",
					Lines:     []string{"北陸新幹線"},
					Positions: []rail.Point{
						{Latitude: 37.081, Longitude: 138.249},
						{Latitude: 37.079, Longitude: 138.247},
					},
				},
			},
		},
		{
			name:  "shinkansen plus extras merging Morioka group and adding Shizukuishi",
			types: []string{rail.InstitutionShinkansen},
			extras: []rail.ExtraStation{
				{LineName: "田沢湖線", Operator: "東日本旅客鉄道", StationName: "盛岡", DisplayLineName: "秋田新幹線"},
				{LineName: "田沢湖線", Operator: "東日本旅客鉄道", StationName: "雫石", DisplayLineName: "秋田新幹線"},
			},
			want: []rail.Station{
				{
					GroupCode: "000792",
					Name:      "盛岡",
					Lines:     []string{"東北新幹線", "秋田新幹線"},
					Positions: []rail.Point{
						{Latitude: 39.702, Longitude: 141.136},
						{Latitude: 39.701, Longitude: 141.134},
					},
				},
				{
					GroupCode: "000801",
					Name:      "雫石",
					Lines:     []string{"秋田新幹線"},
					Positions: []rail.Point{{Latitude: 39.689, Longitude: 140.975}},
				},
				{
					GroupCode: "001200",
					Name:      "上越妙高",
					Lines:     []string{"北陸新幹線"},
					Positions: []rail.Point{
						{Latitude: 37.081, Longitude: 138.249},
						{Latitude: 37.079, Longitude: 138.247},
					},
				},
			},
		},
		{
			name:  "extra matching already-selected shinkansen record does not duplicate position",
			types: []string{rail.InstitutionShinkansen},
			extras: []rail.ExtraStation{
				{LineName: "東北新幹線", Operator: "東日本旅客鉄道", StationName: "盛岡", DisplayLineName: "東北新幹線（表示別名）"},
			},
			want: []rail.Station{
				{
					GroupCode: "000792",
					Name:      "盛岡",
					Lines:     []string{"東北新幹線"},
					Positions: []rail.Point{{Latitude: 39.702, Longitude: 141.136}},
				},
				{
					GroupCode: "001200",
					Name:      "上越妙高",
					Lines:     []string{"北陸新幹線"},
					Positions: []rail.Point{
						{Latitude: 37.081, Longitude: 138.249},
						{Latitude: 37.079, Longitude: 138.247},
					},
				},
			},
		},
		{
			name: "same station name with different GroupCode is not merged",
			records: []rail.Record{
				{
					InstitutionType: rail.InstitutionShinkansen,
					LineName:        "南線",
					Operator:        "南鉄道",
					StationName:     "中央",
					StationCode:     "002000_1",
					GroupCode:       "002000",
					Coordinates: []rail.Point{
						{Latitude: 34.0, Longitude: 135.0},
						{Latitude: 34.0, Longitude: 135.5},
					},
				},
				{
					InstitutionType: rail.InstitutionShinkansen,
					LineName:        "北線",
					Operator:        "北鉄道",
					StationName:     "中央",
					StationCode:     "001000_1",
					GroupCode:       "001000",
					Coordinates: []rail.Point{
						{Latitude: 39.0, Longitude: 141.0},
						{Latitude: 39.0, Longitude: 141.5},
					},
				},
			},
			types: []string{rail.InstitutionShinkansen},
			want: []rail.Station{
				{
					GroupCode: "001000",
					Name:      "中央",
					Lines:     []string{"北線"},
					Positions: []rail.Point{{Latitude: 39.0, Longitude: 141.25}},
				},
				{
					GroupCode: "002000",
					Name:      "中央",
					Lines:     []string{"南線"},
					Positions: []rail.Point{{Latitude: 34.0, Longitude: 135.25}},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recs := tt.records
			if recs == nil {
				recs = sampleRecords
			}
			beforeRecs := snapshot(t, recs)
			got, err := rail.Select(recs, tt.types, tt.extras)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertStationsEqual(t, got, tt.want)

			rev := slices.Clone(recs)
			slices.Reverse(rev)
			gotRev, err := rail.Select(rev, tt.types, tt.extras)
			if err != nil {
				t.Fatalf("reverse unexpected error: %v", err)
			}
			assertStationsEqual(t, gotRev, tt.want)

			if !bytes.Equal(beforeRecs, snapshot(t, recs)) {
				t.Fatal("input records mutated")
			}
		})
	}
}

func assertStationsEqual(t testing.TB, got, want []rail.Station) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d (got=%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i].GroupCode != want[i].GroupCode || got[i].Name != want[i].Name || !slices.Equal(got[i].Lines, want[i].Lines) {
			t.Fatalf("station[%d]=%+v want=%+v", i, got[i], want[i])
		}
		if len(got[i].Positions) != len(want[i].Positions) {
			t.Fatalf("station[%d].Positions=%+v want=%+v", i, got[i].Positions, want[i].Positions)
		}
		for j := range want[i].Positions {
			if math.Abs(got[i].Positions[j].Latitude-want[i].Positions[j].Latitude) > 1e-12 ||
				math.Abs(got[i].Positions[j].Longitude-want[i].Positions[j].Longitude) > 1e-12 {
				t.Fatalf("station[%d].Positions[%d]=%+v want=%+v", i, j, got[i].Positions[j], want[i].Positions[j])
			}
		}
	}
}

func TestSelectErrors(t *testing.T) {
	records, err := rail.ReadGeoJSON(bytes.NewReader(readTestdata(t, "testdata/n02_station_sample.geojson")))
	if err != nil {
		t.Fatal(err)
	}

	conflictRecords := []rail.Record{
		{
			InstitutionType: "1",
			LineName:        "東北新幹線",
			Operator:        "東日本旅客鉄道",
			StationName:     "盛岡",
			StationCode:     "000792_1",
			GroupCode:       "000792",
			Coordinates:     []rail.Point{{Latitude: 39.701, Longitude: 141.135}, {Latitude: 39.703, Longitude: 141.137}},
		},
		{
			InstitutionType: "1",
			LineName:        "別路線",
			Operator:        "東日本旅客鉄道",
			StationName:     "盛岡別名",
			StationCode:     "000792_2",
			GroupCode:       "000792",
			Coordinates:     []rail.Point{{Latitude: 39.701, Longitude: 141.135}, {Latitude: 39.703, Longitude: 141.137}},
		},
	}

	for _, tt := range []struct {
		name    string
		records []rail.Record
		types   []string
		extras  []rail.ExtraStation
		wantIs  error
		wantKey string
	}{
		{
			name:    "extra station not found returns which tuple",
			records: records,
			types:   []string{rail.InstitutionShinkansen},
			extras: []rail.ExtraStation{
				{LineName: "田沢湖線", Operator: "東日本旅客鉄道", StationName: "存在しない駅", DisplayLineName: "秋田新幹線"},
			},
			wantIs:  rail.ErrExtraNotFound,
			wantKey: "田沢湖線/東日本旅客鉄道/存在しない駅",
		},
		{
			name:    "conflicting station names in same group",
			records: conflictRecords,
			types:   []string{rail.InstitutionShinkansen},
			wantIs:  rail.ErrInvalidFormat,
			wantKey: "000792",
		},
		{
			name:    "duplicate institution type",
			records: records,
			types:   []string{"1", "1"},
			wantIs:  rail.ErrInvalidArgument,
		},
		{
			name:    "empty extra field",
			records: records,
			types:   []string{"1"},
			extras:  []rail.ExtraStation{{LineName: "", Operator: "東日本旅客鉄道", StationName: "盛岡"}},
			wantIs:  rail.ErrInvalidArgument,
		},
		{
			name:    "duplicate extra station",
			records: records,
			types:   []string{"1"},
			extras: []rail.ExtraStation{
				{LineName: "田沢湖線", Operator: "東日本旅客鉄道", StationName: "雫石"},
				{LineName: "田沢湖線", Operator: "東日本旅客鉄道", StationName: "雫石"},
			},
			wantIs: rail.ErrInvalidArgument,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rail.Select(tt.records, tt.types, tt.extras)
			if err == nil || got != nil {
				t.Fatalf("got=%+v err=%v want error", got, err)
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("err=%v wantIs=%v", err, tt.wantIs)
			}
			if tt.wantKey != "" {
				var re *rail.ReadError
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

func TestNearest(t *testing.T) {
	const earthRadiusKM = 6371.0088

	for _, tt := range []struct {
		name     string
		stations []rail.Station
		point    rail.Point
		wantCode string
		wantDist float64
		tol      float64
	}{
		{
			name: "same point is 0 km",
			stations: []rail.Station{
				{GroupCode: "000001", Name: "A", Lines: []string{"L1"}, Positions: []rail.Point{{Latitude: 35.0, Longitude: 139.0}}},
			},
			point:    rail.Point{Latitude: 35.0, Longitude: 139.0},
			wantCode: "000001",
			wantDist: 0,
			tol:      0,
		},
		{
			name: "equator 1 degree matches R*pi/180",
			stations: []rail.Station{
				{GroupCode: "000001", Name: "A", Lines: []string{"L1"}, Positions: []rail.Point{{Latitude: 0, Longitude: 1}}},
			},
			point:    rail.Point{Latitude: 0, Longitude: 0},
			wantCode: "000001",
			wantDist: earthRadiusKM * math.Pi / 180,
			tol:      1e-9,
		},
		{
			name: "antipode matches R*pi",
			stations: []rail.Station{
				{GroupCode: "000001", Name: "A", Lines: []string{"L1"}, Positions: []rail.Point{{Latitude: 0, Longitude: 180}}},
			},
			point:    rail.Point{Latitude: 0, Longitude: 0},
			wantCode: "000001",
			wantDist: earthRadiusKM * math.Pi,
			tol:      1e-9,
		},
		{
			name: "uses closest position within station group",
			stations: []rail.Station{
				{
					GroupCode: "000010",
					Name:      "FarSingle",
					Lines:     []string{"L1"},
					Positions: []rail.Point{{Latitude: 0, Longitude: 2}},
				},
				{
					GroupCode: "000020",
					Name:      "MultiRecord",
					Lines:     []string{"L1"},
					Positions: []rail.Point{{Latitude: 0, Longitude: 5}, {Latitude: 0, Longitude: 1}},
				},
			},
			point:    rail.Point{Latitude: 0, Longitude: 0},
			wantCode: "000020",
			wantDist: earthRadiusKM * math.Pi / 180,
			tol:      1e-9,
		},
		{
			name: "tie breaks by smaller GroupCode regardless of input order",
			stations: []rail.Station{
				{GroupCode: "000002", Name: "B", Lines: []string{"L1"}, Positions: []rail.Point{{Latitude: 0, Longitude: 1}}},
				{GroupCode: "000001", Name: "A", Lines: []string{"L1"}, Positions: []rail.Point{{Latitude: 0, Longitude: -1}}},
			},
			point:    rail.Point{Latitude: 0, Longitude: 0},
			wantCode: "000001",
			wantDist: earthRadiusKM * math.Pi / 180,
			tol:      1e-9,
		},
		{
			name: "same station name with different GroupCode returns closer station",
			stations: []rail.Station{
				{GroupCode: "001000", Name: "中央", Lines: []string{"北線"}, Positions: []rail.Point{{Latitude: 0, Longitude: 5}}},
				{GroupCode: "002000", Name: "中央", Lines: []string{"南線"}, Positions: []rail.Point{{Latitude: 0, Longitude: 1}}},
			},
			point:    rail.Point{Latitude: 0, Longitude: 0},
			wantCode: "002000",
			wantDist: earthRadiusKM * math.Pi / 180,
			tol:      1e-9,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := snapshot(t, tt.stations)
			got, err := rail.Nearest(tt.stations, tt.point)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Station.GroupCode != tt.wantCode {
				t.Fatalf("GroupCode=%q want=%q", got.Station.GroupCode, tt.wantCode)
			}
			if math.Abs(got.DistanceKM-tt.wantDist) > tt.tol {
				t.Fatalf("DistanceKM=%.12f want=%.12f (tol=%g)", got.DistanceKM, tt.wantDist, tt.tol)
			}

			// 返り値を書き換えても入力に影響しないことを確認する。
			if len(got.Station.Lines) > 0 {
				got.Station.Lines[0] = "mutated"
			}
			if len(got.Station.Positions) > 0 {
				got.Station.Positions[0] = rail.Point{}
			}
			if !bytes.Equal(before, snapshot(t, tt.stations)) {
				t.Fatal("input stations mutated")
			}
		})
	}
}

func TestNearestErrors(t *testing.T) {
	validStation := rail.Station{
		GroupCode: "000001",
		Name:      "A",
		Lines:     []string{"L1"},
		Positions: []rail.Point{{Latitude: 35.0, Longitude: 139.0}},
	}
	for _, tt := range []struct {
		name     string
		stations []rail.Station
		point    rail.Point
	}{
		{
			name:     "empty stations",
			stations: nil,
			point:    rail.Point{Latitude: 35.0, Longitude: 139.0},
		},
		{
			name:     "point out of range",
			stations: []rail.Station{validStation},
			point:    rail.Point{Latitude: 91.0, Longitude: 139.0},
		},
		{
			name:     "point NaN",
			stations: []rail.Station{validStation},
			point:    rail.Point{Latitude: math.NaN(), Longitude: 139.0},
		},
		{
			name: "station with no positions",
			stations: []rail.Station{
				{GroupCode: "000001", Name: "A", Lines: []string{"L1"}},
			},
			point: rail.Point{Latitude: 35.0, Longitude: 139.0},
		},
		{
			name: "duplicate group code",
			stations: []rail.Station{
				validStation,
				validStation,
			},
			point: rail.Point{Latitude: 35.0, Longitude: 139.0},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rail.Nearest(tt.stations, tt.point)
			if !errors.Is(err, rail.ErrInvalidArgument) {
				t.Fatalf("got=%+v err=%v want ErrInvalidArgument", got, err)
			}
		})
	}
}
