package jma

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

var testElements = []Element{ElementTemperature, ElementDailyMaximum, ElementDailyMinimum, ElementSunshineDuration, ElementMaximumSnowDepth, ElementSnowfallTotal}
var testIDs = []string{"49236", "49251", "50066", "50477"}

type testEntry struct {
	name string
	data []byte
}

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func memoryZIP(t testing.TB, entries []testEntry) ZIPSource {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, entry := range entries {
		f, err := w.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return ZIPSource{Reader: bytes.NewReader(b.Bytes()), Size: int64(b.Len())}
}

func monthlyPath(id string) string { return "monthly/area" + id[:2] + "/nml_amd_m_" + id + ".csv" }

func fixtureEntries(t testing.TB) ([]testEntry, []testEntry) {
	t.Helper()
	m := []testEntry{{name: "unrelated-directory/"}}
	for _, id := range slices.Backward(testIDs) {
		m = append(m, testEntry{name: monthlyPath(id), data: fixture(t, "nml_amd_m_"+id+".csv")})
	}
	i := []testEntry{{name: "ignored/"}, {name: "amedas_station_index.csv", data: fixture(t, "amedas_station_index.csv")}}
	return m, i
}

func firstRows(t testing.TB) ([]byte, []byte) {
	t.Helper()
	for _, row := range bytes.Split(fixture(t, "nml_amd_m_49236.csv"), []byte{'\n'}) {
		if len(row) >= 13 && string(row[9:13]) == "0500" {
			index := bytes.Split(fixture(t, "amedas_station_index.csv"), []byte{'\n'})[2]
			return bytes.Clone(row), bytes.Clone(index)
		}
	}
	t.Fatal("temperature row missing")
	return nil, nil
}

func indexCSV(rows ...[]byte) []byte {
	parts := [][]byte{[]byte(stationHeaders[0]), []byte(stationHeaders[1])}
	parts = append(parts, rows...)
	return append(bytes.Join(parts, []byte{'\n'}), '\n')
}

func readRows(t testing.TB, monthlyRow, stationRow []byte) (*Dataset, error) {
	t.Helper()
	return Read(memoryZIP(t, []testEntry{{monthlyPath("49236"), monthlyRow}}),
		memoryZIP(t, []testEntry{{"amedas_station_index.csv", indexCSV(stationRow)}}), []Element{ElementTemperature})
}

func replaceCol(row []byte, col int, value string) []byte {
	cols := bytes.Split(row, []byte{','})
	cols[col] = []byte(value)
	return bytes.Join(cols, []byte{','})
}

func checkReadError(t testing.TB, ds *Dataset, err error, archive, file string, line int, id string, element Element, field string) *ReadError {
	t.Helper()
	if ds != nil || err == nil || !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("dataset=%v error=%v, want nil and ErrInvalidFormat", ds, err)
	}
	var re *ReadError
	if !errors.As(err, &re) {
		t.Fatalf("not ReadError: %v", err)
	}
	if re.Archive != archive || re.File != file || re.Line != line || re.StationID != id || re.Element != element || re.Field != field {
		t.Fatalf("location=%+v, want %s %s %d %s %s %s", *re, archive, file, line, id, element, field)
	}
	for _, part := range []string{archive, file, id, string(element), field, re.Err.Error()} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q omits %q", err, part)
		}
	}
	if line != 0 && !strings.Contains(err.Error(), fmt.Sprint(line)) {
		t.Errorf("error omits line %d: %v", line, err)
	}
	return re
}

func TestRead(t *testing.T) {
	m, i := fixtureEntries(t)
	for _, elements := range [][]Element{testElements, {ElementSnowfallTotal, ElementTemperature}, {ElementMaximumSnowDepth}} {
		t.Run(fmt.Sprint(elements), func(t *testing.T) {
			input := slices.Clone(elements)
			monthly := &closeTrackingReader{ReaderAt: memoryZIP(t, m).Reader}
			index := &closeTrackingReader{ReaderAt: memoryZIP(t, i).Reader}
			ds, err := Read(ZIPSource{monthly, memoryZIP(t, m).Size}, ZIPSource{index, memoryZIP(t, i).Size}, elements)
			if err != nil {
				t.Fatal(err)
			}
			if ds.Edition != 2020 || len(ds.Stations) != 4 || monthly.closed || index.closed || !slices.Equal(elements, input) {
				t.Fatalf("dataset=%+v closed=%v,%v elements=%v", ds, monthly.closed, index.closed, elements)
			}
			sorted := slices.Clone(elements)
			slices.Sort(sorted)
			units := map[Element]Unit{ElementTemperature: UnitDeciCelsius, ElementDailyMaximum: UnitDeciCelsius, ElementDailyMinimum: UnitDeciCelsius, ElementSunshineDuration: UnitDeciHour, ElementMaximumSnowDepth: UnitCentimeter, ElementSnowfallTotal: UnitCentimeter}
			for j, station := range ds.Stations {
				if station.Station.ID != testIDs[j] || len(station.Normals) != len(elements) {
					t.Fatalf("station=%+v", station)
				}
				for k, n := range station.Normals {
					if n.Element != sorted[k] || n.Unit != units[n.Element] {
						t.Errorf("normal=%+v", n)
					}
					values := append(n.Monthly[:], n.Annual)
					for _, v := range values {
						if v.Status == StatusUnset || v.Status != map[int]Status{0: StatusUnavailable, 8: StatusOK}[v.RMK] {
							t.Errorf("value=%+v", v)
						}
					}
				}
			}
		})
	}
	if (Value{}).Status == StatusOK {
		t.Fatal("zero value is OK")
	}
}

type closeTrackingReader struct {
	io.ReaderAt
	closed bool
}

func (r *closeTrackingReader) Close() error { r.closed = true; return nil }

func TestReadArguments(t *testing.T) {
	m, i := fixtureEntries(t)
	monthly, index := memoryZIP(t, m), memoryZIP(t, i)
	var typedNil *bytes.Reader
	for _, tc := range []struct {
		name string
		m, i ZIPSource
		e    []Element
	}{
		{"nil monthly", ZIPSource{}, index, testElements},
		{"nil index", monthly, ZIPSource{}, testElements},
		{"typed nil", ZIPSource{typedNil, 1}, index, testElements},
		{"zero size", ZIPSource{monthly.Reader, 0}, index, testElements},
		{"negative size", monthly, ZIPSource{index.Reader, -1}, testElements},
		{"empty", monthly, index, nil},
		{"unknown", monthly, index, []Element{"2200"}},
		{"duplicate", monthly, index, []Element{ElementTemperature, ElementTemperature}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, err := Read(tc.m, tc.i, tc.e)
			if ds != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("dataset=%v error=%v", ds, err)
			}
			var re *ReadError
			if errors.As(err, &re) {
				t.Fatalf("argument error is ReadError: %v", err)
			}
		})
	}
}

func TestReadArchiveLayout(t *testing.T) {
	m, i := fixtureEntries(t)
	for _, tc := range []struct {
		name                     string
		m, i                     []testEntry
		archive, file, id, field string
	}{
		{"monthly absent", nil, i, "monthly", "", "", "File"},
		{"index absent", m, nil, "index", "", "", "File"},
		{"index basename", m, []testEntry{{"dir/amedas_station_index.csv", i[1].data}}, "index", "dir/amedas_station_index.csv", "", "File"},
		{"duplicate index", m, []testEntry{i[1], i[1]}, "index", "amedas_station_index.csv", "", "File"},
		{"extra index file", m, append(slices.Clone(i), testEntry{"z.txt", nil}), "index", "amedas_station_index.csv", "", "File"},
		{"extra monthly file", append(slices.Clone(m), testEntry{"z.txt", nil}), i, "monthly", "z.txt", "", "File"},
		{"basename only", []testEntry{{"nml_amd_m_49236.csv", m[4].data}}, i, "monthly", "nml_amd_m_49236.csv", "", "File"},
		{"leading slash", []testEntry{{"/" + monthlyPath("49236"), m[4].data}}, i, "monthly", "/" + monthlyPath("49236"), "", "File"},
		{"prefix dot", []testEntry{{"./" + monthlyPath("49236"), m[4].data}}, i, "monthly", "./" + monthlyPath("49236"), "", "File"},
		{"wrong area", []testEntry{{"monthly/area50/nml_amd_m_49236.csv", m[4].data}}, i, "monthly", "monthly/area50/nml_amd_m_49236.csv", "49236", "File"},
		{"duplicate monthly", append(slices.Clone(m), m[4]), i, "monthly", monthlyPath("49236"), "49236", "File"},
		{"only index station", m[:4], i, "index", "amedas_station_index.csv", "49236", "ID"},
		{"only monthly station", m, []testEntry{{"amedas_station_index.csv", indexCSV(bytes.Split(i[1].data, []byte{'\n'})[2])}}, "monthly", monthlyPath("49251"), "49251", "ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, err := Read(memoryZIP(t, tc.m), memoryZIP(t, tc.i), testElements)
			checkReadError(t, ds, err, tc.archive, tc.file, 0, tc.id, "", tc.field)
		})
	}
}

func TestReadLineEndings(t *testing.T) {
	m, i := fixtureEntries(t)
	base, err := Read(memoryZIP(t, m), memoryZIP(t, i), testElements)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		convert func([]byte) []byte
	}{
		{"LF", bytes.Clone},
		{"CRLF", func(b []byte) []byte { return bytes.ReplaceAll(b, []byte{'\n'}, []byte{'\r', '\n'}) }},
		{"no final newline", func(b []byte) []byte { return bytes.TrimSuffix(b, []byte{'\n'}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mm, ii := slices.Clone(m), slices.Clone(i)
			for j := range mm {
				mm[j].data = tc.convert(mm[j].data)
			}
			for j := range ii {
				ii[j].data = tc.convert(ii[j].data)
			}
			ds, err := Read(memoryZIP(t, mm), memoryZIP(t, ii), testElements)
			if err != nil || !reflect.DeepEqual(ds, base) {
				t.Fatalf("dataset=%v error=%v", ds, err)
			}
		})
	}
}

type payloadFailReader struct {
	io.ReaderAt
	offset int64
	err    error
}

func (r payloadFailReader) ReadAt(p []byte, off int64) (int, error) {
	if off == r.offset {
		n, _ := r.ReaderAt.ReadAt(p[:min(5, len(p))], off)
		return n, r.err
	}
	return r.ReaderAt.ReadAt(p, off)
}

func TestReadZIPFailures(t *testing.T) {
	row, station := firstRows(t)
	sources := [2]ZIPSource{
		memoryZIP(t, []testEntry{{monthlyPath("49236"), row}}),
		memoryZIP(t, []testEntry{{"amedas_station_index.csv", indexCSV(station)}}),
	}
	ioErr := errors.New("payload read failed")
	for archive, source := range sources {
		name := []string{"monthly", "index"}[archive]
		z, err := zip.NewReader(source.Reader, source.Size)
		if err != nil {
			t.Fatal(err)
		}
		offset, err := z.File[0].DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name   string
			modify func() ZIPSource
			want   error
			file   bool
		}{
			{"invalid ZIP", func() ZIPSource { return ZIPSource{bytes.NewReader([]byte("invalid")), 7} }, zip.ErrFormat, false},
			{"I/O", func() ZIPSource { return ZIPSource{payloadFailReader{source.Reader, offset, ioErr}, source.Size} }, ioErr, true},
			{"CRC", func() ZIPSource {
				b := make([]byte, source.Size)
				if _, err := source.Reader.ReadAt(b, 0); err != nil {
					t.Fatal(err)
				}
				b[offset] ^= 1
				return ZIPSource{bytes.NewReader(b), int64(len(b))}
			}, zip.ErrChecksum, true},
			{"unsupported compression", func() ZIPSource {
				b := make([]byte, source.Size)
				if _, err := source.Reader.ReadAt(b, 0); err != nil {
					t.Fatal(err)
				}
				central := bytes.Index(b, []byte{'P', 'K', 1, 2})
				b[8], b[central+10] = 99, 99
				return ZIPSource{bytes.NewReader(b), int64(len(b))}
			}, zip.ErrAlgorithm, true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				inputs := sources
				inputs[archive] = tc.modify()
				ds, err := Read(inputs[0], inputs[1], []Element{ElementTemperature})
				if ds != nil || !errors.Is(err, tc.want) || errors.Is(err, ErrInvalidFormat) {
					t.Fatalf("dataset=%v error=%v, want %v", ds, err, tc.want)
				}
				var re *ReadError
				if !errors.As(err, &re) || re.Archive != name || re.Line != 0 || re.Field != "File" {
					t.Fatalf("location=%+v error=%v", re, err)
				}
				if tc.file && re.File != z.File[0].Name {
					t.Fatalf("file=%s", re.File)
				}
			})
		}
	}
}

type closeErrorReader struct {
	io.Reader
	err error
}

func (r closeErrorReader) Close() error { return r.err }

func TestReadFileCloseError(t *testing.T) {
	row, station := firstRows(t)
	closeErr := errors.New("decompressor close failed")
	for _, name := range []string{"monthly", "index"} {
		t.Run(name, func(t *testing.T) {
			entry := testEntry{monthlyPath("49236"), row}
			if name == "index" {
				entry = testEntry{"amedas_station_index.csv", indexCSV(station)}
			}
			source := memoryZIP(t, []testEntry{entry})
			z, err := zip.NewReader(source.Reader, source.Size)
			if err != nil {
				t.Fatal(err)
			}
			z.RegisterDecompressor(zip.Store, func(r io.Reader) io.ReadCloser { return closeErrorReader{r, closeErr} })
			if name == "index" {
				stations, readErr := readStations(z.File[0])
				err = readErr
				if stations != nil {
					t.Fatal("partial stations returned")
				}
			} else {
				normals, readErr := readNormals(z.File[0], "49236", map[Element]bool{ElementTemperature: true}, []Element{ElementTemperature})
				err = readErr
				if normals != nil {
					t.Fatal("partial normals returned")
				}
			}
			var re *ReadError
			if !errors.Is(err, closeErr) || !errors.As(err, &re) || re.Archive != name || re.File != entry.name || re.Line != 0 || re.Field != "File" {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func ExampleRead() {
	makeSource := func(paths []string, monthly bool) (ZIPSource, error) {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, path := range paths {
			data, err := os.ReadFile("testdata/" + path)
			if err != nil {
				return ZIPSource{}, errors.Join(err, writer.Close())
			}
			name := path
			if monthly {
				id := strings.TrimSuffix(strings.TrimPrefix(path, "nml_amd_m_"), ".csv")
				name = "monthly/area" + id[:2] + "/" + path
			}
			file, err := writer.Create(name)
			if err != nil {
				return ZIPSource{}, errors.Join(err, writer.Close())
			}
			if _, err := file.Write(data); err != nil {
				return ZIPSource{}, errors.Join(err, writer.Close())
			}
		}
		if err := writer.Close(); err != nil {
			return ZIPSource{}, err
		}
		return ZIPSource{Reader: bytes.NewReader(buffer.Bytes()), Size: int64(buffer.Len())}, nil
	}
	monthly, err := makeSource([]string{"nml_amd_m_49236.csv", "nml_amd_m_49251.csv", "nml_amd_m_50066.csv", "nml_amd_m_50477.csv"}, true)
	if err != nil {
		fmt.Println(err)
		return
	}
	index, err := makeSource([]string{"amedas_station_index.csv"}, false)
	if err != nil {
		fmt.Println(err)
		return
	}
	dataset, err := Read(monthly, index, []Element{ElementDailyMinimum})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(dataset.Edition, len(dataset.Stations))
	station := dataset.Stations[0]
	january := station.Normals[0].Monthly[0]
	fmt.Println(station.Station.Name, january.Integer, station.Normals[0].Unit, january.Status == StatusOK)
	// Output:
	// 2020 4
	// 切石 -34 0.1℃ true
}

func TestReadFullZIP(t *testing.T) {
	dir := os.Getenv("JMA_TEST_ZIP_DIR")
	if dir == "" {
		t.Skip("JMA_TEST_ZIP_DIR is not set")
	}
	var sources [2]ZIPSource
	for k, tc := range []struct{ name, hash string }{
		{"normal_amedas_monthly.zip", "0c9e10f7a04df02ff94c5bd57326b8b2f089464dd0d9c9e98d3d9f3b2233a776"},
		{"amedas_station_index.zip", "b5f453a7520ac74aec525862bcc17fee84e4d1eef72c69c614e7d3cd4ef75db8"},
	} {
		f, err := os.Open(filepath.Join(dir, tc.name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := f.Close(); err != nil {
				t.Error(err)
			}
		})
		info, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			t.Fatal(err)
		}
		if hash := fmt.Sprintf("%x", h.Sum(nil)); hash != tc.hash {
			t.Fatalf("%s SHA-256=%s, want %s", tc.name, hash, tc.hash)
		}
		sources[k] = ZIPSource{f, info.Size()}
	}
	ds, err := Read(sources[0], sources[1], testElements)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Edition != 2020 || len(ds.Stations) != 1298 {
		t.Fatalf("edition=%d stations=%d", ds.Edition, len(ds.Stations))
	}
	rmk := map[int]int{}
	marks := [5][3]int{}
	for _, station := range ds.Stations {
		if len(station.Normals) != 6 {
			t.Fatalf("station %s: %d normals", station.Station.ID, len(station.Normals))
		}
		for _, normal := range station.Normals {
			for _, v := range append(normal.Monthly[:], normal.Annual) {
				rmk[v.RMK]++
			}
		}
		s := station.Station
		for k, mark := range []ObservationMark{s.Precipitation, s.Wind, s.Temperature, s.Sunshine, s.SnowDepth} {
			marks[k][mark]++
		}
	}
	if want := map[int]int{0: 46134, 8: 55110}; !reflect.DeepEqual(rmk, want) {
		t.Fatalf("selected RMK=%v, want %v", rmk, want)
	}
	if want := [5][3]int{{1, 1297, 0}, {374, 924, 0}, {373, 925, 0}, {452, 159, 687}, {961, 337, 0}}; marks != want {
		t.Fatalf("observation marks=%v, want %v", marks, want)
	}
	z, err := zip.NewReader(sources[0].Reader, sources[0].Size)
	if err != nil {
		t.Fatal(err)
	}
	elements, allRMK := map[string]int{}, map[string]int{}
	files, dirs, rows := 0, 0, 0
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			dirs++
			continue
		}
		files++
		data, err := fileBytes(f, ReadError{Archive: "monthly", File: f.Name})
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		err = eachLine(data, func(line int, row []byte) error {
			cols, err := columns(row, 143, monthlyWidths)
			if err != nil {
				return fmt.Errorf("%s:%d: %w", f.Name, line, err)
			}
			if string(cols[0]) != "21" {
				return fmt.Errorf("%s:%d: type %s", f.Name, line, cols[0])
			}
			elements[string(cols[2])]++
			for k := range 13 {
				allRMK[string(cols[7+2*k])]++
			}
			count++
			rows++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if count != 122 {
			t.Fatalf("%s: %d rows", f.Name, count)
		}
	}
	if files != 1298 || dirs != 64 || rows != 158356 || len(elements) != 122 {
		t.Fatalf("files=%d dirs=%d rows=%d elements=%d", files, dirs, rows, len(elements))
	}
	for element, count := range elements {
		if count != 1298 {
			t.Errorf("element %s: %d rows", element, count)
		}
	}
	if want := map[string]int{"0": 780348, "8": 1278123, "7": 157}; !reflect.DeepEqual(allRMK, want) {
		t.Fatalf("all RMK=%v, want %v", allRMK, want)
	}
	iz, err := zip.NewReader(sources[1].Reader, sources[1].Size)
	if err != nil {
		t.Fatal(err)
	}
	data, err := fileBytes(iz.File[0], ReadError{Archive: "index", File: iz.File[0].Name})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	err = eachLine(data, func(line int, row []byte) error {
		count++
		if line <= 2 {
			if len(row) != []int{196, 62}[line-1] || !bytes.Equal(row, []byte(stationHeaders[line-1])) {
				return fmt.Errorf("header %d: unexpected bytes", line)
			}
			return nil
		}
		_, err := columns(row, 111, stationWidths)
		return err
	})
	if err != nil || count != 1300 {
		t.Fatalf("index rows=%d error=%v", count, err)
	}
	t.Logf("1298 stations, 6 elements, 13 periods; selected RMK=%v; all 158356 rows and observation marks matched", rmk)
}
