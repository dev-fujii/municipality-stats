package jma

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/japanese"
)

var (
	digitsPattern  = regexp.MustCompile(`^[0-9]+$`)
	integerPattern = regexp.MustCompile(`^-?[0-9]+$`)
)

func readAllAndClose(r io.ReadCloser) ([]byte, error) {
	data, readErr := io.ReadAll(r)
	return data, errors.Join(readErr, r.Close())
}

func eachLine(data []byte, visit func(int, []byte) error) error {
	for line := 1; len(data) > 0; line++ {
		end := bytes.IndexByte(data, '\n')
		var row []byte
		if end < 0 {
			row, data = data, nil
		} else {
			row, data = data[:end], data[end+1:]
			row = bytes.TrimSuffix(row, []byte{'\r'})
		}
		if err := visit(line, row); err != nil {
			return err
		}
	}
	return nil
}

func columns(row []byte, length int, widths []int) ([][]byte, error) {
	if len(row) != length {
		return nil, invalid("line length %d, want %d", len(row), length)
	}
	cols := bytes.Split(row, []byte{','})
	if len(cols) != len(widths) {
		return nil, invalid("column count %d, want %d", len(cols), len(widths))
	}
	for i, w := range widths {
		if len(cols[i]) != w || bytes.ContainsRune(cols[i], '"') {
			return nil, invalid("column %d: expected %d bytes without quotes", i+1, w)
		}
	}
	return cols, nil
}

func decodeSJIS(raw []byte) (string, error) {
	decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(raw)
	if err != nil {
		return "", errors.Join(ErrInvalidFormat, err)
	}
	if bytes.Contains(decoded, []byte("\ufffd")) {
		return "", invalid("invalid Shift_JIS or replacement character")
	}
	if bytes.ContainsRune(decoded, '\\') {
		return "", invalid("escape characters are not supported")
	}
	return string(decoded), nil
}

func parseInteger(s string, signed, leftSpaces bool) (int64, error) {
	if leftSpaces {
		s = strings.TrimLeft(s, " ")
	}
	pattern := digitsPattern
	if signed {
		pattern = integerPattern
	}
	if !pattern.MatchString(s) {
		_, err := strconv.ParseInt(s, 10, 64)
		return 0, errors.Join(invalid("invalid integer %q", s), err)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.Join(ErrInvalidFormat, err)
	}
	return n, nil
}
