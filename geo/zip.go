package geo

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"regexp"
)

// ZIPSource の Reader の開閉は呼び出す側が行う。
type ZIPSource struct {
	Name   string
	Reader io.ReaderAt
	Size   int64
}

var n03Path = regexp.MustCompile(`^N03-[0-9]{8}_[0-9]{2}\.geojson$`)

func validateSources(sources []ZIPSource) error {
	if len(sources) == 0 {
		return fmt.Errorf("%w: sources must not be empty", ErrInvalidArgument)
	}
	seen := make(map[string]bool, len(sources))
	for _, src := range sources {
		if src.Name == "" || seen[src.Name] || nilReader(src.Reader) || src.Size <= 0 {
			return fmt.Errorf("%w: source %q requires a unique name, non-nil Reader and positive Size", ErrInvalidArgument, src.Name)
		}
		seen[src.Name] = true
	}
	return nil
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

func n03Bytes(src ZIPSource) ([]byte, ReadError, error) {
	where := ReadError{Source: src.Name}
	z, err := zip.NewReader(src.Reader, src.Size)
	if err != nil {
		return nil, where, where.fail("File", err)
	}
	var selected *zip.File
	for _, f := range z.File {
		if f.FileInfo().IsDir() || !n03Path.MatchString(path.Base(f.Name)) {
			continue
		}
		where.File = f.Name
		if selected != nil {
			return nil, where, where.fail("File", invalid("multiple N03 GeoJSON files"))
		}
		selected = f
	}
	if selected == nil {
		return nil, where, where.fail("File", invalid("N03 GeoJSON file missing"))
	}
	r, err := selected.Open()
	if err != nil {
		return nil, where, where.fail("File", err)
	}
	data, err := readAllAndClose(r)
	if err != nil {
		return nil, where, where.fail("File", err)
	}
	return data, where, nil
}

func readAllAndClose(r io.ReadCloser) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err = errors.Join(err, r.Close()); err != nil {
		return nil, err
	}
	return data, nil
}
