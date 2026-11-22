package jma

import (
	"bytes"
	"errors"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseInteger(t *testing.T) {
	for _, tc := range []struct {
		text           string
		signed, spaces bool
		want           int64
	}{
		{"000008", false, false, 8}, {"  -34", true, true, -34}, {" -0", true, true, 0},
		{"9223372036854775807", true, false, math.MaxInt64}, {"-9223372036854775808", true, false, math.MinInt64},
	} {
		n, err := parseInteger(tc.text, tc.signed, tc.spaces)
		if err != nil || n != tc.want {
			t.Errorf("%q: n=%d error=%v", tc.text, n, err)
		}
	}
	for _, s := range []string{"", "   ", "1 ", "1 2", "+1", "1.0", "1e3", "1_000", "0x10", "\t1", "--1", "１", "9223372036854775808", "-9223372036854775809"} {
		t.Run(s, func(t *testing.T) {
			_, err := parseInteger(s, true, true)
			if !errors.Is(err, ErrInvalidFormat) {
				t.Fatalf("error=%v", err)
			}
			if strings.Contains(s, "922337203685477580") {
				var number *strconv.NumError
				if !errors.As(err, &number) || !errors.Is(err, strconv.ErrRange) {
					t.Fatalf("lost ParseInt error: %v", err)
				}
			}
		})
	}
	for _, tc := range []struct {
		text           string
		signed, spaces bool
	}{{"-1", false, true}, {" 1", true, false}} {
		if _, err := parseInteger(tc.text, tc.signed, tc.spaces); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("accepted %+v", tc)
		}
	}
}

func TestColumns(t *testing.T) {
	monthly, index := firstRows(t)
	for _, tc := range []struct {
		name   string
		row    []byte
		length int
		widths []int
	}{{"monthly", monthly, 143, monthlyWidths}, {"index", index, 111, stationWidths}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := columns(tc.row, tc.length, tc.widths); err != nil {
				t.Fatal(err)
			}
			for col := 0; col < len(tc.widths)-1; col++ {
				cols := bytes.Split(tc.row, []byte{','})
				cols[col] = append(bytes.Clone(cols[col]), cols[col+1][0])
				cols[col+1] = cols[col+1][1:]
				if _, err := columns(bytes.Join(cols, []byte{','}), tc.length, tc.widths); !errors.Is(err, ErrInvalidFormat) {
					t.Errorf("accepted shifted boundary after column %d", col)
				}
			}
		})
	}
}

func TestEachLine(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"", nil}, {"a\n", []string{"a"}}, {"a\r\nb\r\n", []string{"a", "b"}},
		{"a\r\r\n", []string{"a\r"}}, {"a\r", []string{"a\r"}}, {"a\n\nb", []string{"a", "", "b"}},
	} {
		var got []string
		err := eachLine([]byte(tc.input), func(line int, row []byte) error {
			if line != len(got)+1 {
				t.Errorf("line=%d", line)
			}
			got = append(got, string(row))
			return nil
		})
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: lines=%q error=%v", tc.input, got, err)
		}
	}
	want := errors.New("visitor failed")
	if err := eachLine([]byte("a\nb"), func(int, []byte) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestDecodeSJIS(t *testing.T) {
	for _, tc := range []struct {
		raw  []byte
		want string
	}{
		{[]byte{0x90, 0xd8, 0x90, 0xce}, "切石"}, {[]byte{0xb7, 0xd8, 0xb2, 0xbc}, "ｷﾘｲｼ"}, {[]byte{0x95, 0x5c}, "表"},
	} {
		got, err := decodeSJIS(tc.raw)
		if err != nil || got != tc.want {
			t.Errorf("%x: %q %v", tc.raw, got, err)
		}
	}
	for _, b := range [][]byte{{0x81}, {0x81, 0x7f}, {0xff}, {0xef, 0xbf, 0xbd}, {'\\'}} {
		if _, err := decodeSJIS(b); !errors.Is(err, ErrInvalidFormat) {
			t.Errorf("accepted %x: %v", b, err)
		}
	}
}

type failingReadCloser struct {
	readErr, closeErr error
	closed            bool
}

func (r *failingReadCloser) Read(p []byte) (int, error) { return copy(p, "partial"), r.readErr }
func (r *failingReadCloser) Close() error               { r.closed = true; return r.closeErr }

func TestReadAllAndClose(t *testing.T) {
	readErr, closeErr := errors.New("read failed"), errors.New("close failed")
	for _, tc := range []struct{ read, close error }{{io.EOF, nil}, {readErr, nil}, {io.EOF, closeErr}, {readErr, closeErr}} {
		r := &failingReadCloser{readErr: tc.read, closeErr: tc.close}
		data, err := readAllAndClose(r)
		if string(data) != "partial" || !r.closed {
			t.Fatalf("data=%q closed=%v", data, r.closed)
		}
		for _, want := range []error{tc.read, tc.close} {
			if want != nil && want != io.EOF && !errors.Is(err, want) {
				t.Errorf("lost %v: %v", want, err)
			}
		}
		if tc.read == io.EOF && tc.close == nil && err != nil {
			t.Fatal(err)
		}
	}
}
