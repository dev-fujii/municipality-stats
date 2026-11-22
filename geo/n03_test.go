package geo

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const testN03File = "N03-20260101_10.geojson"

type testEntry struct {
	name string
	data []byte
}

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func memoryZIP(t testing.TB, name string, entries ...testEntry) ZIPSource {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		f, err := w.CreateHeader(&zip.FileHeader{Name: e.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return ZIPSource{Name: name, Reader: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func sourceJSON(t testing.TB, data []byte) ZIPSource {
	t.Helper()
	return memoryZIP(t, "test", testEntry{testN03File, data})
}

func readJSON(t testing.TB, data []byte, rollup map[string]string) *Boundaries {
	t.Helper()
	b, err := ReadN03([]ZIPSource{sourceJSON(t, data)}, rollup)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func document(features ...string) []byte {
	return []byte(`{"type":"FeatureCollection","crs":{"type":"name","properties":{"name":"EPSG:6668"}},"features":[` + strings.Join(features, ",") + `]}`)
}

func feature(code, coordinates string) string {
	return fmt.Sprintf(`{"type":"Feature","properties":{"N03_007":%q},"geometry":{"type":"Polygon","coordinates":%s}}`, code, coordinates)
}

const square = `[[[139,36],[140,36],[140,37],[139,37],[139,36]]]`

func checkReadFailure(t testing.TB, b *Boundaries, err error, sentinel error, source, file, key, field string) {
	t.Helper()
	var where *ReadError
	if b != nil || !errors.Is(err, sentinel) || !errors.As(err, &where) {
		t.Fatalf("got boundaries=%v error=%v, want nil and ReadError wrapping %v", b, err, sentinel)
	}
	if where.Source != source || where.File != file || where.Key != key || where.Field != field || where.Line != 0 {
		t.Fatalf("location=%+v, want source=%q file=%q key=%q field=%q line=0", where, source, file, key, field)
	}
}

func TestReadN03(t *testing.T) {
	for _, tt := range []struct {
		name, file string
		codes      []string
	}{
		{"rectangles", "rectangles.geojson", []string{"10001", "10002"}},
		{"N03 excerpt", "n03-gunma.geojson", []string{"10202", "10464"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src := memoryZIP(t, "fixture", testEntry{"ignored/", nil}, testEntry{"metadata.xml", []byte("ignored")}, testEntry{"nested/" + testN03File, fixture(t, tt.file)})
			b, err := ReadN03([]ZIPSource{src}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(b.AreaCodes(), tt.codes) {
				t.Fatalf("codes=%v, want %v", b.AreaCodes(), tt.codes)
			}
			codes, meshes := b.AreaCodes(), b.PrimaryMeshes()
			codes[0] = "00000"
			meshes[0] = "0000"
			if !slices.Equal(b.AreaCodes(), tt.codes) || b.PrimaryMeshes()[0] == "0000" {
				t.Fatal("returned slices share storage")
			}
		})
	}
}

func TestReadN03SourcesAndRollup(t *testing.T) {
	valid := sourceJSON(t, document(feature("10001", square)))
	var typedNil *bytes.Reader
	for _, tt := range []struct {
		name    string
		sources []ZIPSource
		rollup  map[string]string
	}{
		{"nil sources", nil, nil}, {"empty sources", []ZIPSource{}, nil},
		{"empty name", []ZIPSource{{Reader: valid.Reader, Size: valid.Size}}, nil},
		{"nil reader", []ZIPSource{{Name: "a", Size: 1}}, nil},
		{"typed nil reader", []ZIPSource{{Name: "a", Reader: typedNil, Size: 1}}, nil},
		{"zero size", []ZIPSource{{Name: "a", Reader: valid.Reader}}, nil},
		{"negative size", []ZIPSource{{Name: "a", Reader: valid.Reader, Size: -1}}, nil},
		{"duplicate name", []ZIPSource{valid, valid}, nil},
		{"bad key", []ZIPSource{valid}, map[string]string{"1000": "10000"}},
		{"bad value", []ZIPSource{valid}, map[string]string{"10001": "1000x"}},
		{"self", []ZIPSource{valid}, map[string]string{"10001": "10001"}},
		{"chain", []ZIPSource{valid}, map[string]string{"10001": "10002", "10002": "10000"}},
		{"cycle", []ZIPSource{valid}, map[string]string{"10001": "10002", "10002": "10001"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, err := ReadN03(tt.sources, tt.rollup)
			if b != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("got %v, %v", b, err)
			}
		})
	}
	second := valid
	second.Name = "second"
	rollup := map[string]string{"10001": "10000"}
	b, err := ReadN03([]ZIPSource{second, valid}, rollup)
	if err != nil {
		t.Fatal(err)
	}
	rollup["10001"] = "99999"
	if b.areas[0].code != "10000" || len(b.areas[0].polygons) != 2 || valid.Name != "test" {
		t.Fatal("sources or rollup changed")
	}
}

func TestReadN03InvalidFormat(t *testing.T) {
	base := string(document(feature("10001", square)))
	for _, tt := range []struct{ name, from, to, field, key string }{
		{"collection type", "FeatureCollection", "Feature", "Type", ""},
		{"CRS missing", `"crs"`, `"other"`, "CRS", ""},
		{"CRS wrong", "EPSG:6668", "EPSG:4326", "CRS", ""},
		{"CRS type", `"type":"name"`, `"type":"link"`, "CRS", ""},
		{"features missing", `"features"`, `"other"`, "Features", ""},
		{"features null", `"features":[` + feature("10001", square) + `]`, `"features":null`, "Features", ""},
		{"feature type", `"type":"Feature"`, `"type":"other"`, "Features[0].Type", ""},
		{"missing code", "N03_007", "other", "Features[0].Properties.N03_007", ""},
		{"numeric code", `"10001"`, `10001`, "Features[0].Properties.N03_007", ""},
		{"short code", "10001", "1001", "Features[0].Properties.N03_007", "1001"},
		{"nondigit code", "10001", "1000x", "Features[0].Properties.N03_007", "1000x"},
		{"geometry type", "Polygon", "Point", "Features[0].Geometry", "10001"},
		{"empty polygon", square, `[]`, "Features[0].Geometry", "10001"},
		{"null polygon", square, `null`, "Features[0].Geometry", "10001"},
		{"empty ring", square, `[[]]`, "Features[0].Geometry", "10001"},
		{"position null", "[139,36]", "null", "Features[0].Geometry", "10001"},
		{"position 3D", "[139,36]", "[139,36,0]", "Features[0].Geometry", "10001"},
		{"string coordinate", "[139,36]", `["139",36]`, "Features[0].Geometry", "10001"},
		{"null coordinate", "[139,36]", `[null,36]`, "Features[0].Geometry", "10001"},
		{"longitude range", "[139,36]", "[181,36]", "Features[0].Geometry", "10001"},
		{"latitude range", "[139,36]", "[139,-91]", "Features[0].Geometry", "10001"},
		{"precision", "[139,36]", "[139.0000000000000001,36]", "Features[0].Geometry", "10001"},
		{"overflow", "[139,36]", "[1e30,36]", "Features[0].Geometry", "10001"},
		{"unclosed", square, `[[[139,36],[140,36],[140,37],[139,37]]]`, "Features[0].Geometry", "10001"},
		{"too few", square, `[[[139,36],[140,36],[139,36]]]`, "Features[0].Geometry", "10001"},
		{"distinct points", square, `[[[139,36],[140,36],[139,36],[140,36],[139,36]]]`, "Features[0].Geometry", "10001"},
		{"zero area", square, `[[[139,36],[140,36],[141,36],[139,36]]]`, "Features[0].Geometry", "10001"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(strings.Replace(base, tt.from, tt.to, 1))
			b, err := ReadN03([]ZIPSource{sourceJSON(t, data)}, nil)
			checkReadFailure(t, b, err, ErrInvalidFormat, "test", testN03File, tt.key, tt.field)
		})
	}
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"malformed", []byte(`{"type":`)},
		{"trailing JSON", append([]byte(base), []byte(` {}`)...)},
		{"trailing junk", append([]byte(base), '!')},
		{"invalid UTF8", append([]byte(base), 0xff)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, err := ReadN03([]ZIPSource{sourceJSON(t, tt.data)}, nil)
			checkReadFailure(t, b, err, ErrInvalidFormat, "test", testN03File, "", "JSON")
			if tt.name == "malformed" && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("lost original JSON error")
			}
		})
	}
	bad := sourceJSON(t, document(feature("10002", `[]`)))
	bad.Name = "bad"
	b, err := ReadN03([]ZIPSource{sourceJSON(t, []byte(base)), bad}, nil)
	checkReadFailure(t, b, err, ErrInvalidFormat, "bad", testN03File, "10002", "Features[0].Geometry")
}

func TestReadN03PrecisionAndWinding(t *testing.T) {
	for _, coordinates := range []string{
		`[[[139.123456789012345,36],[140,36],[140,37],[139.123456789012345,37],[139.123456789012345,36]]]`,
		`[[[139.123456789012345000,36],[139.123456789012345000,37],[140,37],[140,36],[139.123456789012345000,36]]]`,
		`[[[139123456789012345e-15,36],[140,36],[140,37],[139123456789012345e-15,37],[139123456789012345e-15,36]]]`,
	} {
		t.Run(coordinates, func(t *testing.T) {
			b := readJSON(t, document(feature("10001", coordinates)), nil)
			if b.areas[0].box.minX != 3*139123456789012345 {
				t.Fatal("precision lost")
			}
		})
	}
}

func TestReadN03FullZIP(t *testing.T) {
	dir := os.Getenv("GEO_TEST_ZIP_DIR")
	if dir == "" {
		t.Skip("GEO_TEST_ZIP_DIR is not set")
	}
	data, err := os.ReadFile(filepath.Join(dir, "N03-20260101_10_GML.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "0981c3412b098ff2a9947e7a8bc57bc353275c2c337b76f7adea52c08b0d7b07" {
		t.Fatalf("SHA-256=%s", got)
	}
	src := ZIPSource{Name: "N03-20260101_10_GML.zip", Reader: bytes.NewReader(data), Size: int64(len(data))}
	start := time.Now()
	b, err := ReadN03([]ZIPSource{src}, nil)
	readTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.AreaCodes()) != 35 {
		t.Fatalf("areas=%d", len(b.AreaCodes()))
	}
	polygons, rings, vertices := 0, 0, 0
	for _, a := range b.areas {
		for _, p := range a.polygons {
			polygons++
			for _, r := range p.rings {
				rings++
				vertices += len(r.points)
			}
		}
	}
	if polygons != 37 || rings != 38 || vertices != 124161 {
		t.Fatalf("polygons=%d rings=%d vertices=%d", polygons, rings, vertices)
	}
	raw, _, err := n03Bytes(src)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := decodeJSON(raw, &root); err != nil {
		t.Fatal(err)
	}
	features := root["features"].([]any)
	maxDigits := 0
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case json.Number:
			_, fraction, _ := strings.Cut(string(x), ".")
			maxDigits = max(maxDigits, len(fraction))
		case []any:
			for _, child := range x {
				visit(child)
			}
		}
	}
	for _, f := range features {
		visit(f.(map[string]any)["geometry"].(map[string]any)["coordinates"])
	}
	if len(features) != 37 || maxDigits != 15 {
		t.Fatalf("features=%d max decimal places=%d", len(features), maxDigits)
	}
	point := rationalPoint("36.3895", "139.0634")
	start = time.Now()
	location, err := b.Locate(point)
	locateTime := time.Since(start)
	if err != nil || location != (Location{AreaCode: "10201"}) {
		t.Fatalf("location=%+v error=%v", location, err)
	}
	// 短い1回の時間が時計の分解能を下回る環境でも、1点あたりの時間を残す。
	const repeats = 10000
	start = time.Now()
	for range repeats {
		if got, err := b.Locate(point); err != nil || got != location {
			t.Fatalf("location=%+v error=%v", got, err)
		}
	}
	averageTime := time.Since(start) / repeats
	t.Logf("features=%d polygons=%d rings=%d vertices=%d max_decimal_places=%d ReadN03=%s Locate_first=%s Locate_average=%s (%d repeats)", len(features), polygons, rings, vertices, maxDigits, readTime, locateTime, averageTime, repeats)
}

func TestReadN03ExcerptProperties(t *testing.T) {
	var root struct {
		Features []struct{ Properties map[string]any }
	}
	if err := json.Unmarshal(fixture(t, "n03-gunma.geojson"), &root); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		index      int
		code, name string
	}{{0, "10202", "高崎市"}, {1, "10464", "玉村町"}} {
		t.Run(tt.code, func(t *testing.T) {
			p := root.Features[tt.index].Properties
			if p["N03_001"] != "群馬県" || p["N03_004"] != tt.name || p["N03_005"] != nil || p["N03_007"] != tt.code {
				t.Fatalf("properties=%v", p)
			}
		})
	}
}

func TestReadN03SourceOrder(t *testing.T) {
	a := sourceJSON(t, document(feature("10002", square)))
	b := sourceJSON(t, document(feature("10001", `[[[140,36],[141,36],[141,37],[140,37],[140,36]]]`)))
	b.Name = "second"
	first, err := ReadN03([]ZIPSource{a, b}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReadN03([]ZIPSource{b, a}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("source order changes result")
	}
}
