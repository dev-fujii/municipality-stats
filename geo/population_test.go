package geo

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/japanese"
)

const testPopulationFile = "tblT001141H5439.txt"

func populationRecords(t testing.TB) [][]string {
	t.Helper()
	decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(fixture(t, testPopulationFile))
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(decoded)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func populationText(t testing.TB, records [][]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.WriteAll(records); err != nil {
		t.Fatal(err)
	}
	raw, err := japanese.ShiftJIS.NewEncoder().Bytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func populationSource(t testing.TB, data []byte) ZIPSource {
	t.Helper()
	return memoryZIP(t, "population", testEntry{testPopulationFile, data})
}

func checkPopulationFailure(t testing.TB, cells []PopulationCell, err, cause error, source, file string, line int, key, field string) {
	t.Helper()
	var where *ReadError
	if cells != nil || !errors.Is(err, cause) || !errors.As(err, &where) {
		t.Fatalf("cells=%v error=%v, want nil and ReadError wrapping %v", cells, err, cause)
	}
	if where.Source != source || where.File != file || where.Line != line || where.Key != key || where.Field != field {
		t.Fatalf("location=%+v", where)
	}
}

func TestReadPopulation(t *testing.T) {
	raw := fixture(t, testPopulationFile)
	want := []PopulationCell{{"543900013", 13}, {"543900041", 22}, {"543900042", 10}}
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"original CRLF", raw},
		{"LF", bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))},
		{"no final newline", bytes.TrimSuffix(raw, []byte("\r\n"))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cells, err := ReadPopulation([]ZIPSource{populationSource(t, tt.data)})
			if err != nil || !slices.Equal(cells, want) {
				t.Fatalf("cells=%v error=%v", cells, err)
			}
		})
	}
	for _, tt := range []struct {
		name   string
		change func([][]string) [][]string
		want   []PopulationCell
	}{
		{"reverse rows", func(r [][]string) [][]string { slices.Reverse(r[2:]); return r }, want},
		{"zero population", func(r [][]string) [][]string { r[2][4] = "0"; return r }, []PopulationCell{{"543900013", 0}, want[1], want[2]}},
		{"leading zero", func(r [][]string) [][]string { r[2][4] = "00013"; return r }, want},
		{"int64 maximum", func(r [][]string) [][]string { r[2][4] = "9223372036854775807"; return r }, []PopulationCell{{"543900013", math.MaxInt64}, want[1], want[2]}},
		{"unused fields", func(r [][]string) [][]string {
			r[1][5], r[2][1], r[2][2], r[2][3], r[2][5] = "別の項目", "X", "543900042", "9999", "-"
			return r
		}, want},
		{"quoted comma and newline", func(r [][]string) [][]string { r[2][5] = "別,項目\n次の行"; return r }, want},
		{"no population rows", func(r [][]string) [][]string { return r[:2] }, []PopulationCell{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cells, err := ReadPopulation([]ZIPSource{populationSource(t, populationText(t, tt.change(populationRecords(t))))})
			if err != nil || cells == nil || !slices.Equal(cells, tt.want) {
				t.Fatalf("cells=%v error=%v", cells, err)
			}
		})
	}
}

func TestReadPopulationArguments(t *testing.T) {
	valid := populationSource(t, fixture(t, testPopulationFile))
	var typedNil *bytes.Reader
	for _, tt := range []struct {
		name    string
		sources []ZIPSource
	}{
		{"nil sources", nil},
		{"empty sources", []ZIPSource{}},
		{"empty name", []ZIPSource{{Reader: valid.Reader, Size: valid.Size}}},
		{"nil reader", []ZIPSource{{Name: "a", Size: 1}}},
		{"typed nil reader", []ZIPSource{{Name: "a", Reader: typedNil, Size: 1}}},
		{"zero size", []ZIPSource{{Name: "a", Reader: valid.Reader}}},
		{"negative size", []ZIPSource{{Name: "a", Reader: valid.Reader, Size: -1}}},
		{"duplicate name", []ZIPSource{valid, valid}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cells, err := ReadPopulation(tt.sources)
			if cells != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("cells=%v error=%v", cells, err)
			}
		})
	}
}

func TestReadPopulationInvalidRecords(t *testing.T) {
	for _, tt := range []struct {
		name, value, field string
		row, column        int
	}{
		{"header key", "MESH", "Header", 0, 0},
		{"header mark", "OTHER", "Header", 0, 1},
		{"header destination", "OTHER", "Header", 0, 2},
		{"header merge", "OTHER", "Header", 0, 3},
		{"wrong population column", "T001141002", "Header", 0, 4},
		{"wrong last column", "T001141049", "Header", 0, 53},
		{"second header leading field", "KEY_CODE", "Header", 1, 0},
		{"second header leading mark", "0", "Header", 1, 1},
		{"second header leading destination", "0", "Header", 1, 2},
		{"second header leading merge", "0", "Header", 1, 3},
		{"missing full width space", "人口（総数）", "Header", 1, 4},
		{"wrong population title", "　人口（男）", "Header", 1, 4},
		{"empty mesh", "", "MeshCode", 2, 0},
		{"short mesh", "54390001", "MeshCode", 2, 0},
		{"long mesh", "5439000131", "MeshCode", 2, 0},
		{"non ASCII mesh", "５43900013", "MeshCode", 2, 0},
		{"letter mesh", "543900a13", "MeshCode", 2, 0},
		{"secondary latitude 8", "543980013", "MeshCode", 2, 0},
		{"secondary longitude 8", "543908013", "MeshCode", 2, 0},
		{"quadrant 0", "543900010", "MeshCode", 2, 0},
		{"quadrant 5", "543900015", "MeshCode", 2, 0},
		{"filename primary mismatch", "544000013", "MeshCode", 2, 0},
		{"empty population", "", "People", 2, 4},
		{"dash", "-", "People", 2, 4},
		{"suppressed", "X", "People", 2, 4},
		{"stars", "***", "People", 2, 4},
		{"negative", "-1", "People", 2, 4},
		{"negative zero", "-0", "People", 2, 4},
		{"plus", "+13", "People", 2, 4},
		{"decimal", "13.0", "People", 2, 4},
		{"exponent", "1e2", "People", 2, 4},
		{"hexadecimal", "0x13", "People", 2, 4},
		{"underscore", "1_3", "People", 2, 4},
		{"left space", " 13", "People", 2, 4},
		{"right space", "13 ", "People", 2, 4},
		{"inner space", "1 3", "People", 2, 4},
		{"full width digits", "１３", "People", 2, 4},
		{"overflow", "9223372036854775808", "People", 2, 4},
		{"duplicate row", "543900013", "MeshCode", 3, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := populationRecords(t)
			r[tt.row][tt.column] = tt.value
			key := ""
			if tt.row >= 2 {
				key = r[tt.row][0]
			}
			cells, err := ReadPopulation([]ZIPSource{populationSource(t, populationText(t, r))})
			checkPopulationFailure(t, cells, err, ErrInvalidFormat, "population", testPopulationFile, tt.row+1, key, tt.field)
			if tt.name == "overflow" {
				var numberErr *strconv.NumError
				if !errors.Is(err, strconv.ErrRange) || !errors.As(err, &numberErr) {
					t.Fatalf("lost numeric cause: %v", err)
				}
			}
		})
	}
}

func TestReadPopulationCSVStructure(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func([][]string) [][]string
		line   int
		field  string
	}{
		{"empty", func(r [][]string) [][]string { return nil }, 1, "Header"},
		{"missing title", func(r [][]string) [][]string { return r[:1] }, 2, "Header"},
		{"short first header", func(r [][]string) [][]string { r[0] = r[0][:53]; return r }, 1, "Header"},
		{"long second header", func(r [][]string) [][]string { r[1] = append(r[1], ""); return r }, 2, "Header"},
		{"short second header", func(r [][]string) [][]string { r[1] = r[1][:53]; return r }, 2, "Header"},
		{"short data", func(r [][]string) [][]string { r[2] = r[2][:53]; return r }, 3, "Line"},
		{"long data", func(r [][]string) [][]string { r[2] = append(r[2], ""); return r }, 3, "Line"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cells, err := ReadPopulation([]ZIPSource{populationSource(t, populationText(t, tt.change(populationRecords(t))))})
			key := ""
			if tt.line == 3 {
				key = "543900013"
			}
			checkPopulationFailure(t, cells, err, ErrInvalidFormat, "population", testPopulationFile, tt.line, key, tt.field)
		})
	}
	raw := fixture(t, testPopulationFile)
	for _, tt := range []struct {
		name, field string
		data        []byte
		line        int
	}{
		{"blank first line", "Header", append([]byte("\n"), raw...), 1},
		{"blank data line", "Line", bytes.Replace(raw, []byte("543900013"), []byte("\r\n543900013"), 1), 3},
		{"trailing blank line", "Line", append(slices.Clone(raw), '\n'), 6},
		{"bare quote", "Line", bytes.Replace(raw, []byte("543900013"), []byte("5439\"0013"), 1), 3},
		{"unterminated quote", "Line", bytes.Replace(raw, []byte("\r\n543900042,"), []byte("\r\n\"543900042,"), 1), 5},
		{"UTF8 BOM", "Encoding", append([]byte{0xef, 0xbb, 0xbf}, raw...), 1},
		{"invalid byte", "Encoding", append(slices.Clone(raw), 0xff), 6},
		{"truncated lead", "Encoding", append(slices.Clone(raw), 0x81), 6},
		{"bad trail", "Encoding", append(slices.Clone(raw), 0x81, 0x30), 6},
		{"replacement from unused field", "Encoding", bytes.Replace(raw, []byte("543900013,0"), []byte{'5', '4', '3', '9', '0', '0', '0', '1', '3', ',', 0xff}, 1), 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cells, err := ReadPopulation([]ZIPSource{populationSource(t, tt.data)})
			checkPopulationFailure(t, cells, err, ErrInvalidFormat, "population", testPopulationFile, tt.line, "", tt.field)
			if strings.Contains(tt.name, "quote") {
				var parseErr *csv.ParseError
				if !errors.As(err, &parseErr) {
					t.Fatalf("lost CSV cause: %v", err)
				}
			}
		})
	}
}

func TestReadPopulationZIP(t *testing.T) {
	raw := fixture(t, testPopulationFile)
	for _, tt := range []struct {
		name    string
		entries []testEntry
		file    string
	}{
		{"no files", nil, ""},
		{"metadata only", []testEntry{{"metadata.xml", []byte("metadata")}}, ""},
		{"wrong filename", []testEntry{{"tblT001141H543.txt", raw}}, ""},
		{"wrong dataset", []testEntry{{"tblT001142H5439.txt", raw}}, ""},
		{"duplicate path", []testEntry{{testPopulationFile, raw}, {testPopulationFile, raw}}, testPopulationFile},
		{"two matches", []testEntry{{testPopulationFile, raw}, {"tblT001141H5440.txt", raw}}, "tblT001141H5440.txt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cells, err := ReadPopulation([]ZIPSource{memoryZIP(t, "zip", tt.entries...)})
			checkPopulationFailure(t, cells, err, ErrInvalidFormat, "zip", tt.file, 0, "", "File")
		})
	}
	t.Run("bad zip", func(t *testing.T) {
		cells, err := ReadPopulation([]ZIPSource{{Name: "bad", Reader: bytes.NewReader([]byte("bad")), Size: 3}})
		checkPopulationFailure(t, cells, err, zip.ErrFormat, "bad", "", 0, "", "File")
	})
	t.Run("crc", func(t *testing.T) {
		src := populationSource(t, raw)
		data := make([]byte, src.Size)
		if _, err := src.Reader.ReadAt(data, 0); err != nil {
			t.Fatal(err)
		}
		index := bytes.Index(data, raw)
		if index < 0 {
			t.Fatal("stored data missing")
		}
		data[index+len(raw)/2] ^= 1
		src.Reader = bytes.NewReader(data)
		cells, err := ReadPopulation([]ZIPSource{src})
		checkPopulationFailure(t, cells, err, zip.ErrChecksum, "population", testPopulationFile, 0, "", "File")
	})
	t.Run("io", func(t *testing.T) {
		src := memoryZIP(t, "population", testEntry{testPopulationFile, raw}, testEntry{"metadata.xml", bytes.Repeat([]byte("padding"), 2000)})
		z, err := zip.NewReader(src.Reader, src.Size)
		if err != nil {
			t.Fatal(err)
		}
		offset, err := z.File[0].DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		failure := errors.New("read failure")
		src.Reader = failingReaderAt{src.Reader, offset, offset + int64(len(raw)), failure}
		cells, err := ReadPopulation([]ZIPSource{src})
		checkPopulationFailure(t, cells, err, failure, "population", testPopulationFile, 0, "", "File")
	})
	t.Run("directories metadata and caller ownership", func(t *testing.T) {
		src := memoryZIP(t, "population", testEntry{testPopulationFile + "/", nil}, testEntry{"metadata.xml", []byte("unused")}, testEntry{"data/" + testPopulationFile, raw})
		r := &closeReaderAt{Reader: src.Reader.(*bytes.Reader)}
		src.Reader = r
		sources := []ZIPSource{src}
		before := slices.Clone(sources)
		cells, err := ReadPopulation(sources)
		if err != nil || len(cells) != 3 || r.closed || !reflect.DeepEqual(sources, before) {
			t.Fatalf("cells=%v error=%v closed=%v sources=%v", cells, err, r.closed, sources)
		}
		if !bytes.Equal(raw, fixture(t, testPopulationFile)) {
			t.Fatal("input changed")
		}
	})
}

func TestReadPopulationMultipleZIPs(t *testing.T) {
	first := populationSource(t, fixture(t, testPopulationFile))
	r := populationRecords(t)
	for _, row := range r[2:] {
		row[0] = "5440" + row[0][4:]
	}
	second := memoryZIP(t, "second", testEntry{"tblT001141H5440.txt", populationText(t, r)})
	for _, sources := range [][]ZIPSource{{first, second}, {second, first}} {
		cells, err := ReadPopulation(sources)
		want := []PopulationCell{{"543900013", 13}, {"543900041", 22}, {"543900042", 10}, {"544000013", 13}, {"544000041", 22}, {"544000042", 10}}
		if err != nil || !slices.Equal(cells, want) {
			t.Fatalf("cells=%v error=%v", cells, err)
		}
	}
	for _, tt := range []struct {
		name, people string
	}{
		{"same values", "13"},
		{"different values", "999"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := populationRecords(t)
			r[2][4] = tt.people
			duplicate := populationSource(t, populationText(t, r))
			duplicate.Name = "duplicate"
			cells, err := ReadPopulation([]ZIPSource{first, duplicate})
			checkPopulationFailure(t, cells, err, ErrInvalidFormat, "duplicate", testPopulationFile, 3, "543900013", "MeshCode")
		})
	}
	bad := populationSource(t, []byte("bad"))
	bad.Name = "bad"
	cells, err := ReadPopulation([]ZIPSource{first, bad})
	checkPopulationFailure(t, cells, err, ErrInvalidFormat, "bad", testPopulationFile, 1, "", "Header")
}

func TestReadPopulationFullZIP(t *testing.T) {
	dir := os.Getenv("GEO_TEST_ZIP_DIR")
	if dir == "" {
		t.Skip("GEO_TEST_ZIP_DIR is not set")
	}
	const name = "tblT001141H5439.zip"
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	archiveHash := fmt.Sprintf("%x", sha256.Sum256(data))
	// 配布 ZIP の時刻が取得ごとに変わるため、記録済みの2版を許し、本文も照合する。
	switch archiveHash {
	case "3163b3b217d22422cff3b631b5a0fe91119a2f54113ae60463fdc63109debe87",
		"73b15df636c53d6e27d80c6def25c582814800c9403df811183e43a5da54196f":
	default:
		t.Fatalf("unrecorded ZIP SHA-256=%s; compare the contents and update testdata/README.md", archiveHash)
	}
	src := ZIPSource{Name: name, Reader: bytes.NewReader(data), Size: int64(len(data))}
	raw, _, err := selectedZIPBytes(src, populationPath, "population text")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "32d3bb761e2dd13c0be08a850ee9f7820c3f4eb7b4f2ca11f81a17de0b93cd5a" {
		t.Fatalf("text SHA-256=%s", got)
	}
	start := time.Now()
	cells, err := ReadPopulation([]ZIPSource{src})
	readTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(decoded)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	marks := make(map[string]int)
	var total, returnedTotal int64
	for _, record := range records[2:] {
		people, err := strconv.ParseInt(record[4], 10, 64)
		if err != nil || people <= 0 {
			t.Fatalf("mesh=%s population=%q error=%v", record[0], record[4], err)
		}
		marks[record[1]]++
		total += people
	}
	for i, cell := range cells {
		if cell.People <= 0 || (i > 0 && cells[i-1].MeshCode >= cell.MeshCode) {
			t.Fatalf("invalid returned cell: %+v", cell)
		}
		returnedTotal += cell.People
	}
	if len(records)-2 != 18334 || len(cells) != 18334 || !reflect.DeepEqual(marks, map[string]int{"0": 15392, "1": 1342, "2": 1600}) || total != 5018881 || returnedTotal != total {
		t.Fatalf("rows=%d cells=%d marks=%v total=%d returned_total=%d", len(records)-2, len(cells), marks, total, returnedTotal)
	}
	t.Logf("ZIP_SHA256=%s rows=%d HTKSYORI=%v all_positive=true total=%d ReadPopulation=%s", archiveHash, len(cells), marks, total, readTime)
}
