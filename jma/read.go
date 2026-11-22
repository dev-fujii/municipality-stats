package jma

import (
	"archive/zip"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// ZIPSource は、呼び出す側が保持する ZIP の Reader とバイト数を持つ。
type ZIPSource struct {
	Reader io.ReaderAt
	Size   int64
}

// Dataset は、平年値の版と観測所番号順の全地点を持つ。
type Dataset struct {
	Edition  int
	Stations []StationNormals
}

// StationNormals は、地点情報と要素番号順の平年値を持つ。
type StationNormals struct {
	Station Station
	Normals []Normal
}

var monthlyPathPattern = regexp.MustCompile(`^monthly/area([0-9]{2})/nml_amd_m_([0-9]{5})\.csv$`)

// Read は、地点情報と月・年別の ZIP から指定した要素を全地点ぶん読む。
// Reader は閉じない。失敗時は Dataset を返さない。
func Read(monthly ZIPSource, index ZIPSource, elements []Element) (*Dataset, error) {
	for _, src := range []ZIPSource{monthly, index} {
		if nilReader(src.Reader) || src.Size <= 0 {
			return nil, fmt.Errorf("%w: Reader must be non-nil and Size positive", ErrInvalidArgument)
		}
	}
	if len(elements) == 0 {
		return nil, fmt.Errorf("%w: elements must not be empty", ErrInvalidArgument)
	}
	wanted := make(map[Element]bool, len(elements))
	for _, e := range elements {
		if _, ok := elementUnit(e); !ok || wanted[e] {
			return nil, fmt.Errorf("%w: unsupported or duplicate element %q", ErrInvalidArgument, e)
		}
		wanted[e] = true
	}
	ordered := slices.Clone(elements)
	slices.Sort(ordered)
	indexFiles, err := archiveFiles(index, "index")
	if err != nil {
		return nil, err
	}
	if len(indexFiles) != 1 || indexFiles[0].Name != "amedas_station_index.csv" {
		where := ReadError{Archive: "index"}
		if len(indexFiles) > 0 {
			where.File = indexFiles[0].Name
		}
		return nil, where.fail("File", invalid("expected exactly amedas_station_index.csv"))
	}
	stations, err := readStations(indexFiles[0])
	if err != nil {
		return nil, err
	}
	monthlyFiles, err := archiveFiles(monthly, "monthly")
	if err != nil {
		return nil, err
	}
	if len(monthlyFiles) == 0 {
		return nil, (ReadError{Archive: "monthly"}).fail("File", invalid("no monthly CSV files"))
	}
	result := &Dataset{Edition: 2020}
	seen := make(map[string]bool, len(monthlyFiles))
	for _, f := range monthlyFiles {
		where := ReadError{Archive: "monthly", File: f.Name}
		match := monthlyPathPattern.FindStringSubmatch(f.Name)
		if match == nil {
			return nil, where.fail("File", invalid("unexpected file path"))
		}
		id := match[2]
		where.StationID = id
		if match[1] != id[:2] || seen[id] {
			return nil, where.fail("File", invalid("area mismatch or duplicate station file"))
		}
		seen[id] = true
		normals, err := readNormals(f, id, wanted, ordered)
		if err != nil {
			return nil, err
		}
		result.Stations = append(result.Stations, StationNormals{Station: stations[id], Normals: normals})
	}
	for _, f := range monthlyFiles {
		id := monthlyPathPattern.FindStringSubmatch(f.Name)[2]
		if _, ok := stations[id]; !ok {
			return nil, (ReadError{Archive: "monthly", File: f.Name, StationID: id}).fail("ID", invalid("station missing from index"))
		}
	}
	ids := make([]string, 0, len(stations))
	for id := range stations {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if !seen[id] {
			return nil, (ReadError{Archive: "index", File: indexFiles[0].Name, StationID: id}).fail("ID", invalid("station missing from monthly archive"))
		}
	}
	slices.SortFunc(result.Stations, func(a, b StationNormals) int { return strings.Compare(a.Station.ID, b.Station.ID) })
	return result, nil
}

func nilReader(r io.ReaderAt) bool {
	if r == nil {
		return true
	}
	v := reflect.ValueOf(r)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func archiveFiles(src ZIPSource, name string) ([]*zip.File, error) {
	z, err := zip.NewReader(src.Reader, src.Size)
	if err != nil {
		return nil, (ReadError{Archive: name}).fail("File", err)
	}
	files := make([]*zip.File, 0, len(z.File))
	for _, f := range z.File {
		if !f.FileInfo().IsDir() {
			files = append(files, f)
		}
	}
	slices.SortFunc(files, func(a, b *zip.File) int { return strings.Compare(a.Name, b.Name) })
	return files, nil
}

func fileBytes(f *zip.File, where ReadError) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, where.fail("File", err)
	}
	data, err := readAllAndClose(r)
	if err != nil {
		return nil, where.fail("File", err)
	}
	return data, nil
}
