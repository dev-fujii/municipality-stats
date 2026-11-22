package geo

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"testing"
)

type failingReaderAt struct {
	io.ReaderAt
	start, end int64
	err        error
}

func (r failingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < r.end && off+int64(len(p)) > r.start {
		return 0, r.err
	}
	return r.ReaderAt.ReadAt(p, off)
}

type closeReaderAt struct {
	*bytes.Reader
	closed bool
}

func (r *closeReaderAt) Close() error { r.closed = true; return nil }

func TestN03ZIP(t *testing.T) {
	validData := document(feature("10001", square))
	for _, tt := range []struct {
		name    string
		entries []testEntry
		file    string
	}{
		{"no files", nil, ""},
		{"metadata only", []testEntry{{"metadata.xml", []byte("metadata")}}, ""},
		{"wrong filename", []testEntry{{"N03.geojson", validData}}, ""},
		{"duplicate path", []testEntry{{testN03File, validData}, {testN03File, validData}}, testN03File},
		{"two matches", []testEntry{{testN03File, validData}, {"N03-20260101_20.geojson", validData}}, "N03-20260101_20.geojson"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, err := ReadN03([]ZIPSource{memoryZIP(t, "zip", tt.entries...)}, nil)
			checkReadFailure(t, b, err, ErrInvalidFormat, "zip", tt.file, "", "File")
		})
	}
	t.Run("bad zip", func(t *testing.T) {
		b, err := ReadN03([]ZIPSource{{Name: "bad", Reader: bytes.NewReader([]byte("bad")), Size: 3}}, nil)
		checkReadFailure(t, b, err, zip.ErrFormat, "bad", "", "", "File")
	})
	t.Run("crc", func(t *testing.T) {
		src := sourceJSON(t, validData)
		data := make([]byte, src.Size)
		if _, err := src.Reader.ReadAt(data, 0); err != nil {
			t.Fatal(err)
		}
		index := bytes.Index(data, validData)
		if index < 0 {
			t.Fatal("stored data missing")
		}
		data[index+len(validData)/2] ^= 1
		src.Reader = bytes.NewReader(data)
		b, err := ReadN03([]ZIPSource{src}, nil)
		checkReadFailure(t, b, err, zip.ErrChecksum, "test", testN03File, "", "File")
	})
	t.Run("io", func(t *testing.T) {
		src := memoryZIP(t, "test", testEntry{testN03File, validData}, testEntry{"metadata.xml", bytes.Repeat([]byte("padding"), 2000)})
		z, err := zip.NewReader(src.Reader, src.Size)
		if err != nil {
			t.Fatal(err)
		}
		offset, err := z.File[0].DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		failure := errors.New("read failure")
		src.Reader = failingReaderAt{src.Reader, offset, offset + int64(len(validData)), failure}
		b, err := ReadN03([]ZIPSource{src}, nil)
		checkReadFailure(t, b, err, failure, "test", testN03File, "", "File")
	})
	t.Run("reader remains open", func(t *testing.T) {
		src := sourceJSON(t, validData)
		r := &closeReaderAt{Reader: src.Reader.(*bytes.Reader)}
		src.Reader = r
		if _, err := ReadN03([]ZIPSource{src}, nil); err != nil {
			t.Fatal(err)
		}
		if r.closed {
			t.Fatal("caller Reader was closed")
		}
	})
}

type failingReadCloser struct {
	io.Reader
	closeErr error
	closed   bool
}

func (r *failingReadCloser) Close() error { r.closed = true; return r.closeErr }

func TestReadAllAndClose(t *testing.T) {
	readErr, closeErr := errors.New("read"), errors.New("close")
	for _, tt := range []struct {
		name                          string
		reader                        io.Reader
		closeErr, wantRead, wantClose error
	}{
		{"close", bytes.NewReader([]byte("data")), closeErr, nil, closeErr},
		{"read and close", errorReader{readErr}, closeErr, readErr, closeErr},
		{"success", bytes.NewReader([]byte("data")), nil, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &failingReadCloser{Reader: tt.reader, closeErr: tt.closeErr}
			data, err := readAllAndClose(r)
			if !r.closed {
				t.Fatal("not closed")
			}
			if tt.wantRead != nil && !errors.Is(err, tt.wantRead) {
				t.Fatal(err)
			}
			if tt.wantClose != nil && !errors.Is(err, tt.wantClose) {
				t.Fatal(err)
			}
			if tt.wantRead != nil || tt.wantClose != nil {
				if data != nil {
					t.Fatal("partial data returned")
				}
			} else if err != nil || string(data) != "data" {
				t.Fatalf("data=%q err=%v", data, err)
			}
		})
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
