package geo

import (
	"errors"
	"strings"
	"testing"
)

func TestReadError(t *testing.T) {
	cause := errors.New("cause")
	for _, tt := range []struct {
		name      string
		where     ReadError
		fragments []string
	}{
		{"all", ReadError{Source: "source.zip", File: "n03.geojson", Line: 12, Key: "10001", Field: "Features[0].Geometry", Err: errors.Join(ErrInvalidFormat, cause)}, []string{"geo:", "source.zip", "n03.geojson", "line 12", "10001", "Features[0].Geometry", "cause"}},
		{"unknown", ReadError{Err: cause}, []string{"geo:", "cause"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got *ReadError
			if !errors.Is(&tt.where, cause) || !errors.As(&tt.where, &got) || got != &tt.where {
				t.Fatal("error chain lost")
			}
			for _, s := range tt.fragments {
				if !strings.Contains(tt.where.Error(), s) {
					t.Fatalf("missing %q in %q", s, tt.where.Error())
				}
			}
		})
	}
}
