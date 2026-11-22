package geo

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const testElevationFile = "G04-c-11_5439-jgd.xml"

func elevationSource(t testing.TB, data []byte) ZIPSource {
	t.Helper()
	return memoryZIP(t, "elevation", testEntry{testElevationFile, data})
}

func elevationFailure(t testing.TB, sources []ZIPSource, cause error, source, file, field, key string, line int) ReadError {
	t.Helper()
	cells, err := ReadElevation(sources)
	var where *ReadError
	if cells != nil || !errors.Is(err, cause) || !errors.As(err, &where) {
		t.Fatalf("cells=%v error=%v; want nil and ReadError wrapping %v", cells, err, cause)
	}
	if where.Source != source || where.File != file || where.Field != field || where.Key != key || (line >= 0 && where.Line != line) {
		t.Fatalf("location=%+v; want source=%s file=%s field=%s key=%s line=%d", where, source, file, field, key, line)
	}
	if line < 0 && where.Line <= 0 {
		t.Fatalf("missing XML line: %+v", where)
	}
	return *where
}

func elevationLine(data []byte, marker string) int {
	index := bytes.Index(data, []byte(marker))
	if index < 0 {
		panic("line marker missing: " + marker)
	}
	return bytes.Count(data[:index], []byte{'\n'}) + 1
}

func TestReadElevation(t *testing.T) {
	raw := fixture(t, testElevationFile)
	rowStart := bytes.Index(raw, []byte("543900041,"))
	tail := bytes.SplitN(raw[rowStart:], []byte{'\n'}, 3)
	reversed := append(bytes.Clone(raw[:rowStart]), bytes.Join([][]byte{tail[1], tail[0], tail[2]}, []byte{'\n'})...)
	want := []ElevationCell{{"543900013", big.NewRat(2934, 10)}, {"543900041", big.NewRat(3384, 10)}, {"543900042", big.NewRat(3316, 10)}}
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"original BOM", raw},
		{"no BOM", bytes.TrimPrefix(raw, []byte{239, 187, 191})},
		{"LF", bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))},
		{"CRLF", bytes.ReplaceAll(bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))},
		{"no final newline", bytes.TrimRight(raw, "\r\n")},
		{"unsorted rows", reversed},
		{"different namespace prefixes", []byte(strings.NewReplacer("ksj:", "k:", "xmlns:ksj=", "xmlns:k=", "gml:", "g:", "xmlns:gml=", "xmlns:g=").Replace(string(raw)))},
		{"blank lines around tuples", bytes.ReplaceAll(raw, []byte("<gml:tupleList>"), []byte("<gml:tupleList>\n \t\n"))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cells, err := ReadElevation([]ZIPSource{elevationSource(t, tt.data)})
			if err != nil || !reflect.DeepEqual(cells, want) {
				t.Fatalf("cells=%v error=%v", cells, err)
			}
		})
	}
	for _, tt := range []struct {
		value string
		want  *big.Rat
	}{
		{"+123", big.NewRat(123, 1)},
		{"-12.50", big.NewRat(-25, 2)},
		{"0", new(big.Rat)},
		{"-0.0", new(big.Rat)},
		{"+0.0", new(big.Rat)},
		{"001.2500", big.NewRat(5, 4)},
		{"0.1234567890123456789", rationalPoint("0.1234567890123456789", "0").Latitude},
		{"123456789012345678901234567890", rationalPoint("123456789012345678901234567890", "0").Latitude},
		{"unknown", nil},
	} {
		t.Run(tt.value, func(t *testing.T) {
			data := bytes.Replace(raw, []byte("543900013,293.4,"), []byte("543900013,"+tt.value+","), 1)
			cells, err := ReadElevation([]ZIPSource{elevationSource(t, data)})
			if err != nil || len(cells) != 3 || cells[0].MeshCode != "543900013" || (cells[0].Meters == nil) != (tt.want == nil) ||
				(tt.want != nil && cells[0].Meters.Cmp(tt.want) != 0) {
				t.Fatalf("cells=%v error=%v", cells, err)
			}
		})
	}
	t.Run("unused columns", func(t *testing.T) {
		data := bytes.Replace(raw, []byte("363.8,243.1,0,7.7,3,4.4,1,5.4"), []byte("X,,NaN,Inf,unknown,-,?,"), 1)
		cells, err := ReadElevation([]ZIPSource{elevationSource(t, data)})
		if err != nil || !reflect.DeepEqual(cells, want) {
			t.Fatalf("cells=%v error=%v", cells, err)
		}
	})
	t.Run("independent rationals", func(t *testing.T) {
		data := bytes.ReplaceAll(raw, []byte("331.6"), []byte("338.4"))
		src := elevationSource(t, data)
		first, err := ReadElevation([]ZIPSource{src})
		if err != nil {
			t.Fatal(err)
		}
		first[1].Meters.SetInt64(999)
		second, err := ReadElevation([]ZIPSource{src})
		if err != nil || first[2].Meters.Cmp(want[1].Meters) != 0 || second[1].Meters.Cmp(want[1].Meters) != 0 {
			t.Fatalf("first=%v second=%v error=%v", first, second, err)
		}
	})
}

func TestReadElevationInvalidRows(t *testing.T) {
	raw := fixture(t, testElevationFile)
	for _, tt := range []struct {
		name, old, replacement, field, key string
	}{
		{"short columns", "543900013,293.4,", "543900013,", "tupleList", "543900013"},
		{"long columns", "543900013,", "543900013,extra,", "tupleList", "543900013"},
		{"internal blank line", "543900042,", "\n543900042,", "tupleList", ""},
		{"short mesh", "543900013", "54390013", "MeshCode", "54390013"},
		{"long mesh", "543900013", "5439000013", "MeshCode", "5439000013"},
		{"non ASCII mesh", "543900013", "５43900013", "MeshCode", "５43900013"},
		{"non digit mesh", "543900013", "54390001X", "MeshCode", "54390001X"},
		{"secondary latitude", "543900013", "543980013", "MeshCode", "543980013"},
		{"secondary longitude", "543900013", "543908013", "MeshCode", "543908013"},
		{"quadrant zero", "543900013", "543900010", "MeshCode", "543900010"},
		{"quadrant five", "543900013", "543900015", "MeshCode", "543900015"},
		{"primary mismatch", "543900013", "544000013", "MeshCode", "544000013"},
		{"parent mismatch", "543900013", "543900023", "MeshCode", "543900023"},
		{"duplicate row", "543900042", "543900041", "MeshCode", "543900041"},
		{"mesh whitespace", "543900013", " 543900013", "MeshCode", " 543900013"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := bytes.Replace(raw, []byte(tt.old), []byte(tt.replacement), 1)
			line := elevationLine(data, tt.replacement)
			if tt.name == "duplicate row" {
				line = elevationLine(raw, "543900042")
			}
			elevationFailure(t, []ZIPSource{elevationSource(t, data)}, ErrInvalidFormat, "elevation", testElevationFile, tt.field, tt.key, line)
		})
	}
	for _, value := range []string{"", " ", " 1", "1 ", "+", "-", ".5", "1.", "1e2", "1E-2", "NaN", "Inf", "+Inf", "-Inf", "UNKNOWN", "Unknown", "1/2", "0x10", "１", "1_000", "1..0"} {
		t.Run("elevation "+value, func(t *testing.T) {
			data := bytes.Replace(raw, []byte("543900013,293.4,"), []byte("543900013,"+value+","), 1)
			elevationFailure(t, []ZIPSource{elevationSource(t, data)}, ErrInvalidFormat, "elevation", testElevationFile, "Meters", "543900013", elevationLine(data, "543900013"))
		})
	}
}

func TestReadElevationStructure(t *testing.T) {
	raw := fixture(t, testElevationFile)
	for _, tt := range []struct {
		name, old, replacement, field string
	}{
		{"wrong Dataset", "ksj:Dataset", "ksj:Other", "Dataset"},
		{"ksj namespace", ksjNamespace, "urn:other-ksj", "Dataset"},
		{"gml namespace", gmlNamespace, "urn:other-gml", "recordType"},
		{"feature namespace", "ksj:ElevationAndSlopeAngleFourthMesh", "gml:ElevationAndSlopeAngleFourthMesh", "Namespace"},
		{"header namespace", "gml:CompositeValue", "ksj:CompositeValue", "recordType"},
		{"components namespace", "gml:valueComponents", "ksj:valueComponents", "recordType"},
		{"category namespace", "gml:CategoryList", "ksj:CategoryList", "recordType"},
		{"quantity namespace", "gml:QuantityList", "ksj:QuantityList", "recordType"},
		{"record id namespace", `gml:id="recordType"`, `id="recordType"`, "recordType"},
		{"record id", `gml:id="recordType"`, `gml:id="different"`, "recordType"},
		{"uom", `uom="m">平均標高`, `uom="cm">平均標高`, "recordType"},
		{"uom missing", `uom="m">平均標高`, `>平均標高`, "recordType"},
		{"uom namespace", `uom="m">平均標高`, `gml:uom="m">平均標高`, "recordType"},
		{"column kind", "<gml:CategoryList>4次メッシュコード</gml:CategoryList>", "<gml:QuantityList>4次メッシュコード</gml:QuantityList>", "recordType"},
		{"column order", ">平均標高</gml:QuantityList>\r\n\t\t\t<gml:QuantityList uom=\"m\">最高標高<", ">最高標高</gml:QuantityList>\r\n\t\t\t<gml:QuantityList uom=\"m\">平均標高<", "recordType"},
		{"short header", "<gml:QuantityList uom=\"度\">平均傾斜角度</gml:QuantityList>", "", "recordType"},
		{"long header", "</gml:valueComponents>", "<gml:CategoryList>extra</gml:CategoryList></gml:valueComponents>", "recordType"},
		{"missing header", "gml:CompositeValue", "gml:Other", "Dataset"},
		{"missing components", "gml:valueComponents", "gml:Other", "recordType"},
		{"duplicate header", "</gml:CompositeValue>", "</gml:CompositeValue><gml:CompositeValue gml:id=\"recordType\"/>", "recordType"},
		{"range namespace", "gml:rangeSet", "ksj:rangeSet", "rangeSet"},
		{"block namespace", "gml:DataBlock", "ksj:DataBlock", "DataBlock"},
		{"tuple namespace", "gml:tupleList", "ksj:tupleList", "DataBlock"},
		{"parameter namespace", "gml:rangeParameters", "ksj:rangeParameters", "DataBlock"},
		{"reference namespace", xlinkNamespace, "urn:other-xlink", "rangeParameters"},
		{"wrong reference", `xlink:href="#recordType"`, `xlink:href="#grid"`, "rangeParameters"},
		{"external reference", `xlink:href="#recordType"`, `xlink:href="other.xml#recordType"`, "rangeParameters"},
		{"missing reference", `xlink:href="#recordType"`, "", "rangeParameters"},
		{"missing parameters", `<gml:rangeParameters xlink:href="#recordType" />`, "", "DataBlock"},
		{"duplicate parameters", `<gml:rangeParameters xlink:href="#recordType" />`, `<gml:rangeParameters xlink:href="#recordType" /><gml:rangeParameters xlink:href="#recordType" />`, "rangeParameters"},
		{"missing parent", "ksj:tertiaryMeshCode", "ksj:Other", "tertiaryMeshCode"},
		{"parent namespace", "ksj:tertiaryMeshCode", "gml:tertiaryMeshCode", "tertiaryMeshCode"},
		{"short parent", ">54390001<", ">5439001<", "tertiaryMeshCode"},
		{"invalid parent", ">54390001<", ">54398001<", "tertiaryMeshCode"},
		{"duplicate parent", "</ksj:tertiaryMeshCode>", "</ksj:tertiaryMeshCode><ksj:tertiaryMeshCode>54390001</ksj:tertiaryMeshCode>", "tertiaryMeshCode"},
		{"nested scalar", ">54390001<", "><ksj:code>54390001</ksj:code><", "tertiaryMeshCode"},
		{"empty tuple", "543900013,293.4,363.8,243.1,0,7.7,3,4.4,1,5.4", "", "tupleList"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Contains(raw, []byte(tt.old)) {
				t.Fatal("replacement target missing")
			}
			data := bytes.ReplaceAll(raw, []byte(tt.old), []byte(tt.replacement))
			elevationFailure(t, []ZIPSource{elevationSource(t, data)}, ErrInvalidFormat, "elevation", testElevationFile, tt.field, "", -1)
		})
	}
	for _, title := range []string{"4次メッシュコード", "平均標高", "最高標高", "最低標高", "最低標高コード", "最大傾斜角度", "最大傾斜方向", "最小傾斜角度", "最小傾斜方向", "平均傾斜角度"} {
		t.Run("title "+title, func(t *testing.T) {
			data := bytes.Replace(raw, []byte(">"+title+"<"), []byte(">different<"), 1)
			elevationFailure(t, []ZIPSource{elevationSource(t, data)}, ErrInvalidFormat, "elevation", testElevationFile, "recordType", "", -1)
		})
	}
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"truncated XML", raw[:len(raw)/2]},
		{"invalid UTF8", bytes.Replace(raw, []byte("平均標高"), []byte{255}, 1)},
		{"trailing text", append(slices.Clone(raw), 'x')},
		{"broken metadata", bytes.Replace(raw, []byte("</gml:boundedBy>"), []byte("</gml:other>"), 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := elevationFailure(t, []ZIPSource{elevationSource(t, tt.data)}, ErrInvalidFormat, "elevation", testElevationFile, "XML", "", -1)
			if tt.name != "trailing text" {
				var syntax *xml.SyntaxError
				if !errors.As(err.Err, &syntax) {
					t.Fatalf("lost XML cause: %v", err)
				}
			}
		})
	}
}

func TestReadElevationMultipleZIPs(t *testing.T) {
	raw := fixture(t, testElevationFile)
	first := elevationSource(t, raw)
	second := memoryZIP(t, "second", testEntry{"G04-c-11_5440-jgd.xml", bytes.ReplaceAll(raw, []byte("5439"), []byte("5440"))})
	var previous []ElevationCell
	for _, sources := range [][]ZIPSource{{first, second}, {second, first}} {
		cells, err := ReadElevation(sources)
		if err != nil || len(cells) != 6 {
			t.Fatalf("cells=%v error=%v", cells, err)
		}
		if previous != nil && !reflect.DeepEqual(cells, previous) {
			t.Fatal("ZIP order changed results")
		}
		for i := 1; i < len(cells); i++ {
			if cells[i-1].MeshCode >= cells[i].MeshCode {
				t.Fatal("unsorted results")
			}
		}
		previous = cells
	}
	for _, value := range []string{"293.4", "999", "unknown"} {
		t.Run("duplicate "+value, func(t *testing.T) {
			duplicate := elevationSource(t, bytes.Replace(raw, []byte("293.4"), []byte(value), 1))
			duplicate.Name = "duplicate"
			elevationFailure(t, []ZIPSource{first, duplicate}, ErrInvalidFormat, "duplicate", testElevationFile, "MeshCode", "543900013", elevationLine(raw, "543900013"))
		})
	}
	bad := elevationSource(t, []byte("<bad/>"))
	bad.Name = "bad"
	elevationFailure(t, []ZIPSource{first, bad}, ErrInvalidFormat, "bad", testElevationFile, "Dataset", "", 1)
}

func TestReadElevationDataBlocks(t *testing.T) {
	raw := fixture(t, testElevationFile)
	for _, tt := range []struct {
		name, parameters, tuples, field, key string
	}{
		{"valid second block", `<gml:rangeParameters xlink:href="#recordType"/>`, "543900014,10,,,,,,,,", "", ""},
		{"wrong second reference", `<gml:rangeParameters xlink:href="#other"/>`, "543900014,10,,,,,,,,", "rangeParameters", ""},
		{"missing second reference", "", "543900014,10,,,,,,,,", "DataBlock", ""},
		{"duplicate across blocks", `<gml:rangeParameters xlink:href="#recordType"/>`, "543900013,10,,,,,,,,", "MeshCode", "543900013"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			block := "\n<gml:DataBlock>\n" + tt.parameters + "\n<gml:tupleList>" + tt.tuples + "</gml:tupleList>\n</gml:DataBlock>"
			data := bytes.Replace(raw, []byte("</gml:DataBlock>"), []byte("</gml:DataBlock>"+block), 1)
			sources := []ZIPSource{elevationSource(t, data)}
			if tt.field != "" {
				elevationFailure(t, sources, ErrInvalidFormat, "elevation", testElevationFile, tt.field, tt.key, -1)
				return
			}
			cells, err := ReadElevation(sources)
			if err != nil || len(cells) != 4 || cells[1].MeshCode != "543900014" || cells[1].Meters.Cmp(big.NewRat(10, 1)) != 0 {
				t.Fatalf("cells=%v error=%v", cells, err)
			}
		})
	}
}

func TestReadElevationArguments(t *testing.T) {
	valid := elevationSource(t, fixture(t, testElevationFile))
	var typedNil *bytes.Reader
	for _, sources := range [][]ZIPSource{
		nil, {}, {valid, valid},
		{{Name: "", Reader: valid.Reader, Size: valid.Size}},
		{{Name: "nil", Size: 1}},
		{{Name: "typed nil", Reader: typedNil, Size: 1}},
		{{Name: "zero", Reader: valid.Reader}},
		{{Name: "negative", Reader: valid.Reader, Size: -1}},
	} {
		cells, err := ReadElevation(sources)
		if cells != nil || !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("cells=%v error=%v", cells, err)
		}
	}
}

func TestReadElevationZIP(t *testing.T) {
	raw := fixture(t, testElevationFile)
	for _, tt := range []struct {
		name    string
		entries []testEntry
		file    string
	}{
		{"empty", nil, ""},
		{"metadata only", []testEntry{{"KS-META-G04-c.xml", []byte("metadata")}}, ""},
		{"wrong year", []testEntry{{"G04-c-12_5439-jgd.xml", raw}}, ""},
		{"wrong mesh length", []testEntry{{"G04-c-11_543-jgd.xml", raw}}, ""},
		{"wrong suffix", []testEntry{{"G04-c-11_5439.xml", raw}}, ""},
		{"duplicate path", []testEntry{{testElevationFile, raw}, {testElevationFile, raw}}, testElevationFile},
		{"two matches", []testEntry{{testElevationFile, raw}, {"G04-c-11_5440-jgd.xml", raw}}, "G04-c-11_5440-jgd.xml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			elevationFailure(t, []ZIPSource{memoryZIP(t, "zip", tt.entries...)}, ErrInvalidFormat, "zip", tt.file, "File", "", 0)
		})
	}
	t.Run("bad ZIP", func(t *testing.T) {
		elevationFailure(t, []ZIPSource{{Name: "bad", Reader: bytes.NewReader([]byte("bad")), Size: 3}}, zip.ErrFormat, "bad", "", "File", "", 0)
	})
	t.Run("CRC", func(t *testing.T) {
		src := elevationSource(t, raw)
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
		elevationFailure(t, []ZIPSource{src}, zip.ErrChecksum, "elevation", testElevationFile, "File", "", 0)
	})
	t.Run("IO", func(t *testing.T) {
		src := memoryZIP(t, "elevation", testEntry{testElevationFile, raw}, testEntry{"metadata.xml", bytes.Repeat([]byte("padding"), 2000)})
		z, err := zip.NewReader(src.Reader, src.Size)
		if err != nil {
			t.Fatal(err)
		}
		offset, err := z.File[0].DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		cause := errors.New("read failure")
		src.Reader = failingReaderAt{src.Reader, offset, offset + int64(len(raw)), cause}
		elevationFailure(t, []ZIPSource{src}, cause, "elevation", testElevationFile, "File", "", 0)
	})
	t.Run("directories metadata and caller ownership", func(t *testing.T) {
		src := memoryZIP(t, "elevation", testEntry{testElevationFile + "/", nil}, testEntry{"metadata.xml", []byte("unused")}, testEntry{"data/" + testElevationFile, raw})
		r := &closeReaderAt{Reader: src.Reader.(*bytes.Reader)}
		src.Reader = r
		sources := []ZIPSource{src}
		before := slices.Clone(sources)
		cells, err := ReadElevation(sources)
		if err != nil || len(cells) != 3 || r.closed || !reflect.DeepEqual(sources, before) || !bytes.Equal(raw, fixture(t, testElevationFile)) {
			t.Fatalf("cells=%v error=%v closed=%v sources=%v", cells, err, r.closed, sources)
		}
	})
}

func TestReadElevationFullZIP(t *testing.T) {
	dir := os.Getenv("GEO_TEST_ZIP_DIR")
	if dir == "" {
		t.Skip("GEO_TEST_ZIP_DIR is not set")
	}
	const name = "G04-c-11_5439-jgd_GML.zip"
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	src := ZIPSource{Name: name, Reader: bytes.NewReader(data), Size: int64(len(data))}
	raw, _, err := selectedZIPBytes(src, elevationPath, "G04-c XML")
	if err != nil {
		t.Fatal(err)
	}
	contentHash := fmt.Sprintf("%x", sha256.Sum256(raw))
	if contentHash != "e7dbfd8607d12e72a0ef49b188cda0dd365174d69d8b0ae0fdcc573c741b98df" {
		t.Fatalf("content_sha256=%s; compare the contents and update testdata/README.md", contentHash)
	}
	start := time.Now()
	cells, err := ReadElevation([]ZIPSource{src})
	readTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	unknown := 0
	var low, high *big.Rat
	for i, cell := range cells {
		if i > 0 && cells[i-1].MeshCode >= cell.MeshCode {
			t.Fatalf("unsorted mesh %s", cell.MeshCode)
		}
		if cell.Meters == nil {
			unknown++
			continue
		}
		if low == nil || cell.Meters.Cmp(low) < 0 {
			low = cell.Meters
		}
		if high == nil || cell.Meters.Cmp(high) > 0 {
			high = cell.Meters
		}
	}
	if len(cells) != 25600 || unknown != 0 || low == nil || high == nil || low.Cmp(big.NewRat(53, 10)) != 0 || high.Cmp(big.NewRat(18698, 10)) != 0 {
		t.Fatalf("rows=%d unknown=%d min=%v max=%v", len(cells), unknown, low, high)
	}
	t.Logf("zip_sha256=%x content_sha256=%s rows=%d unknown=%d min=%s max=%s ReadElevation=%s", sha256.Sum256(data), contentHash, len(cells), unknown, low.FloatString(1), high.FloatString(1), readTime)
}
