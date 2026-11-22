package geo

import (
	"bytes"
	"cmp"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"

	"golang.org/x/text/encoding/japanese"
)

// PopulationCell は500mメッシュの総人口を持つ。
type PopulationCell struct {
	MeshCode string
	People   int64
}

var populationPath = regexp.MustCompile(`^tblT001141H([0-9]{4})\.txt$`)

// ReadPopulation は各 ZIP の人口を読み、メッシュコード順に返す。
func ReadPopulation(sources []ZIPSource) ([]PopulationCell, error) {
	if err := validateSources(sources); err != nil {
		return nil, err
	}
	cells := make([]PopulationCell, 0)
	seen := make(map[string]bool)
	for _, src := range sources {
		data, where, err := selectedZIPBytes(src, populationPath, "population text")
		if err != nil {
			return nil, err
		}
		primary := populationPath.FindStringSubmatch(path.Base(where.File))[1]
		rows, err := populationRows(data, where)
		if err != nil {
			return nil, err
		}
		for i := 0; ; i++ {
			record, line, err := rows.read()
			where.Line, where.Key = line, ""
			if i >= 2 && len(record) > 0 {
				where.Key = record[0]
			}
			field := "Line"
			if i < 2 {
				field = "Header"
			}
			if err == io.EOF && i >= 2 {
				break
			}
			if err != nil {
				return nil, where.fail(field, errors.Join(ErrInvalidFormat, err))
			}
			if i < 2 {
				if err := populationHeader(record, i); err != nil {
					return nil, where.fail("Header", err)
				}
				continue
			}
			if err := validateMeshCode(record[0]); err != nil {
				return nil, where.fail("MeshCode", err)
			}
			if record[0][:4] != primary {
				return nil, where.fail("MeshCode", invalid("primary mesh differs from filename"))
			}
			if seen[record[0]] {
				return nil, where.fail("MeshCode", invalid("duplicate mesh"))
			}
			people, err := populationInteger(record[4])
			if err != nil {
				return nil, where.fail("People", err)
			}
			seen[record[0]] = true
			cells = append(cells, PopulationCell{record[0], people})
		}
	}
	slices.SortFunc(cells, func(a, b PopulationCell) int { return cmp.Compare(a.MeshCode, b.MeshCode) })
	return cells, nil
}

type populationCSV struct {
	reader *csv.Reader
	data   []byte
	line   int
}

func populationRows(data []byte, where ReadError) (*populationCSV, error) {
	decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(data)
	if err != nil {
		return nil, where.fail("Encoding", errors.Join(ErrInvalidFormat, err))
	}
	if i := bytes.Index(decoded, []byte("\ufffd")); i >= 0 {
		where.Line = bytes.Count(decoded[:i], []byte{'\n'}) + 1
		return nil, where.fail("Encoding", invalid("invalid Shift_JIS or replacement character"))
	}
	r := csv.NewReader(bytes.NewReader(decoded))
	r.FieldsPerRecord = 54
	return &populationCSV{reader: r, data: decoded, line: 1}, nil
}

func (r *populationCSV) read() ([]string, int, error) {
	start, line := r.reader.InputOffset(), r.line
	record, err := r.reader.Read()
	end := r.reader.InputOffset()
	r.line += bytes.Count(r.data[start:end], []byte{'\n'})
	if len(record) > 0 {
		first, _ := r.reader.FieldPos(0)
		if first != line {
			return nil, line, invalid("empty CSV line")
		}
	}
	if err == io.EOF && end > start {
		return nil, line, invalid("empty CSV line")
	}
	var parseErr *csv.ParseError
	if errors.As(err, &parseErr) {
		line = parseErr.Line
	}
	return record, line, err
}

func populationHeader(record []string, index int) error {
	if index == 0 {
		for i, name := range record {
			want := ""
			if i < 4 {
				want = [...]string{"KEY_CODE", "HTKSYORI", "HTKSAKI", "GASSAN"}[i]
			} else {
				want = fmt.Sprintf("T001141%03d", i-3)
			}
			if name != want {
				return invalid("header column %d: got %q, want %q", i+1, name, want)
			}
		}
	} else if record[0] != "" || record[1] != "" || record[2] != "" || record[3] != "" || record[4] != "　人口（総数）" {
		return invalid("population title or leading empty columns differ")
	}
	return nil
}

func populationInteger(s string) (int64, error) {
	if s == "" {
		return 0, invalid("empty population")
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return 0, invalid("population must be a non-negative decimal integer: %q", s)
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.Join(ErrInvalidFormat, err)
	}
	return n, nil
}
