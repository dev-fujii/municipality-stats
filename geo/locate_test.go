package geo

import (
	"errors"
	"math/big"
	"slices"
	"testing"
)

func rationalPoint(latitude, longitude string) Point {
	lat, ok := new(big.Rat).SetString(latitude)
	if !ok {
		panic(latitude)
	}
	lon, ok := new(big.Rat).SetString(longitude)
	if !ok {
		panic(longitude)
	}
	return Point{Latitude: lat, Longitude: lon}
}

func withoutBoxes(b *Boundaries) *Boundaries {
	world := rectangle{-180 * coordinateScale, -90 * coordinateScale, 180 * coordinateScale, 90 * coordinateScale}
	result := &Boundaries{areas: slices.Clone(b.areas)}
	for i := range result.areas {
		a := &result.areas[i]
		a.box = world
		a.polygons = slices.Clone(a.polygons)
		for j := range a.polygons {
			p := &a.polygons[j]
			p.box = world
			p.rings = slices.Clone(p.rings)
			for k := range p.rings {
				p.rings[k].box = world
			}
		}
	}
	return result
}

func TestLocate(t *testing.T) {
	data := fixture(t, "rectangles.geojson")
	for _, rollup := range []map[string]string{nil, {"10001": "10000", "10002": "10000"}} {
		b := readJSON(t, data, rollup)
		unfiltered := withoutBoxes(b)
		for _, tt := range []struct {
			name, lat, lon, code string
			boundary             bool
		}{
			{"inside polygon", "36.1", "139.1", "10002", false},
			{"inside multipolygon", "36.5", "140.5", "10001", false},
			{"multipolygon island", "36.5", "142.5", "10001", false},
			{"same code island", "36.5", "144.5", "10002", false},
			{"hole", "36.5", "139.5", "", false},
			{"hole edge", "36.5", "139.25", "10002", true},
			{"hole horizontal edge", "36.25", "139.5", "10002", true},
			{"hole vertex", "36.25", "139.25", "10002", true},
			{"outer edge", "36.5", "139", "10002", true},
			{"outer horizontal edge", "36", "139.5", "10002", true},
			{"outer vertex", "36", "139", "10002", true},
			{"shared edge", "36.5", "140", "10001", true},
			{"shared vertex", "37", "140", "10001", true},
			{"null code", "36.5", "146.5", "", false},
			{"empty code", "36.5", "148.5", "", false},
			{"outside", "35.5", "139.5", "", false},
			{"between islands", "36.5", "141.5", "", false},
		} {
			t.Run(tt.name, func(t *testing.T) {
				want := Location{tt.code, tt.boundary}
				if to, ok := rollup[want.AreaCode]; ok {
					want.AreaCode = to
				}
				point := rationalPoint(tt.lat, tt.lon)
				beforeLat, beforeLon := point.Latitude.RatString(), point.Longitude.RatString()
				for _, boundaries := range []*Boundaries{b, unfiltered} {
					got, err := boundaries.Locate(point)
					if err != nil || got != want {
						t.Fatalf("got=%+v err=%v, want=%+v", got, err, want)
					}
				}
				if point.Latitude.RatString() != beforeLat || point.Longitude.RatString() != beforeLon {
					t.Fatal("input point changed")
				}
			})
		}
	}
}

func TestLocateOverlap(t *testing.T) {
	data := document(feature("10001", square), feature("10002", square))
	for _, tt := range []struct {
		name    string
		rollup  map[string]string
		want    Location
		invalid bool
	}{
		{"different areas", nil, Location{}, true},
		{"same city", map[string]string{"10001": "10000", "10002": "10000"}, Location{AreaCode: "10000"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := readJSON(t, data, tt.rollup)
			got, err := b.Locate(rationalPoint("36.5", "139.5"))
			if got != tt.want || (tt.invalid && !errors.Is(err, ErrInvalidFormat)) || (!tt.invalid && err != nil) {
				t.Fatalf("got=%+v err=%v", got, err)
			}
			if tt.invalid {
				var where *ReadError
				if !errors.As(err, &where) || where.Key != "10002" || where.Source != "" || where.File != "" || where.Field != "Geometry" {
					t.Fatalf("overlap location missing: %v", err)
				}
			}
		})
	}
	// 同じ区域の別ポリゴンの内部にいても、入力の輪上という情報を残す。
	b := readJSON(t, document(feature("10001", square), feature("10001", `[[[139.5,36],[140.5,36],[140.5,37],[139.5,37],[139.5,36]]]`)), nil)
	got, err := b.Locate(rationalPoint("36.5", "139.5"))
	if err != nil || got != (Location{"10001", true}) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestLocatePrecision(t *testing.T) {
	b := readJSON(t, document(feature("10001", `[[[139.123456789012345,36],[140,36],[140,37],[139.123456789012345,37],[139.123456789012345,36]]]`)), nil)
	for _, tt := range []struct {
		name  string
		delta int64
		want  Location
	}{
		{"one unit west", -1, Location{}},
		{"boundary", 0, Location{"10001", true}},
		{"one unit east", 1, Location{AreaCode: "10001"}},
		{"one decimal step west", -3, Location{}},
		{"one decimal step east", 3, Location{AreaCode: "10001"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := rationalPoint("36.5", "139.123456789012345")
			p.Longitude.Add(p.Longitude, big.NewRat(tt.delta, coordinateScale))
			got, err := b.Locate(p)
			if err != nil || got != tt.want {
				t.Fatalf("got=%+v err=%v, want=%+v", got, err, tt.want)
			}
		})
	}
}

func TestLocateInvalidArgument(t *testing.T) {
	b := readJSON(t, document(feature("10001", square)), nil)
	for _, tt := range []struct {
		name       string
		boundaries *Boundaries
		point      Point
	}{
		{"nil boundaries", nil, rationalPoint("36", "139")},
		{"nil latitude", b, Point{Longitude: big.NewRat(139, 1)}},
		{"nil longitude", b, Point{Latitude: big.NewRat(36, 1)}},
		{"latitude too low", b, rationalPoint("-90.000000000000001", "139")},
		{"latitude too high", b, rationalPoint("91", "139")},
		{"longitude too low", b, rationalPoint("36", "-181")},
		{"longitude too high", b, rationalPoint("36", "180.000000000000001")},
		{"unsupported denominator", b, rationalPoint("1/7", "139")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.boundaries.Locate(tt.point)
			if got != (Location{}) || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
