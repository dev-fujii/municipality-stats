package climate_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dev-fujii/municipality-stats/climate"
	"github.com/dev-fujii/municipality-stats/geo"
	"github.com/dev-fujii/municipality-stats/jma"
)

var elements = []jma.Element{
	jma.ElementTemperature, jma.ElementDailyMaximum, jma.ElementDailyMinimum,
	jma.ElementSunshineDuration, jma.ElementMaximumSnowDepth, jma.ElementSnowfallTotal,
}

func point(lat, lon int64) geo.Point {
	return geo.Point{Latitude: big.NewRat(lat, 1), Longitude: big.NewRat(lon, 1)}
}

func snapshot(t testing.TB, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func boundaries(t testing.TB, overlap bool) *geo.Boundaries {
	t.Helper()
	second := `[[[2,1],[3,1],[3,2],[2,2],[2,1]]]`
	if overlap {
		second = `[[[1,1],[2,1],[2,2],[1,2],[1,1]]]`
	}
	doc := fmt.Sprintf(`{"type":"FeatureCollection","crs":{"type":"name","properties":{"name":"EPSG:6668"}},"features":[
		{"type":"Feature","properties":{"N03_007":"99001"},"geometry":{"type":"Polygon","coordinates":[[[1,1],[2,1],[2,2],[1,2],[1,1]]]}},
		{"type":"Feature","properties":{"N03_007":"99002"},"geometry":{"type":"Polygon","coordinates":%s}},
		{"type":"Feature","properties":{"N03_007":"99003"},"geometry":{"type":"Polygon","coordinates":[[[4.001666666666666,1.001666666666666],[4.001666666666667,1.001666666666666],[4.001666666666667,1.001666666666667],[4.001666666666666,1.001666666666667],[4.001666666666666,1.001666666666666]]]}}
	]}`, second)
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("N03-20260101_99.geojson")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(doc)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := geo.ReadN03([]geo.ZIPSource{{Name: "synthetic", Reader: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}}, map[string]string{"99001": "99000"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func station(id string, lat, lon, altitude int64) jma.Station {
	return jma.Station{ID: id, Latitude: jma.Coordinate{TotalMinutesTenths: lat}, Longitude: jma.Coordinate{TotalMinutesTenths: lon}, AltitudeMeters: altitude}
}

func normal(element jma.Element, status jma.Status) jma.Normal {
	n := jma.Normal{Element: element, Annual: jma.Value{Status: status}}
	if status == jma.StatusOK {
		n.Annual.RMK = 8
	}
	for i := range n.Monthly {
		n.Monthly[i] = jma.Value{Status: jma.StatusUnavailable}
	}
	return n
}

func TestLocateStations(t *testing.T) {
	b := boundaries(t, false)
	for _, tt := range []struct {
		name     string
		stations []jma.Station
		want     []climate.StationLocation
	}{
		{"exact coordinates and rollup", []jma.Station{station("00002", 900, 1500, 0), station("00001", 601, 601, 0)}, []climate.StationLocation{
			{StationID: "00001", Point: geo.Point{Latitude: big.NewRat(601, 600), Longitude: big.NewRat(601, 600)}, AreaCode: "99000"},
			{StationID: "00002", Point: geo.Point{Latitude: big.NewRat(3, 2), Longitude: big.NewRat(5, 2)}, AreaCode: "99002"},
		}},
		{"shared boundary", []jma.Station{station("00001", 900, 1200, 0)}, []climate.StationLocation{
			{StationID: "00001", Point: geo.Point{Latitude: big.NewRat(3, 2), Longitude: big.NewRat(2, 1)}, AreaCode: "99000", OnBoundary: true},
		}},
		{"nationwide outside", []jma.Station{station("00001", 27000, 85000, 0)}, []climate.StationLocation{
			{StationID: "00001", Point: geo.Point{Latitude: big.NewRat(45, 1), Longitude: big.NewRat(425, 3)}},
		}},
		{"unrounded point used for location", []jma.Station{station("00001", 601, 2401, 0)}, []climate.StationLocation{
			{StationID: "00001", Point: geo.Point{Latitude: big.NewRat(601, 600), Longitude: big.NewRat(2401, 600)}, AreaCode: "99003"},
		}},
		{"empty", nil, []climate.StationLocation{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := snapshot(t, tt.stations)
			got, err := climate.LocateStations(b, tt.stations)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got=%+v err=%v want=%+v", got, err, tt.want)
			}
			if !bytes.Equal(before, snapshot(t, tt.stations)) {
				t.Fatal("input changed")
			}
		})
	}
}

func TestDistance(t *testing.T) {
	const r = 6_371_008.8
	for _, tt := range []struct {
		name      string
		a, b      geo.Point
		want      float64
		tolerance float64
	}{
		{"same", point(36, 139), point(36, 139), 0, 0},
		{"equatorial degree", point(0, 0), point(0, 1), r * math.Pi / 180, 1e-8},
		{"antipodes", point(0, 0), point(0, 180), r * math.Pi, 1e-8},
		{"antipodes with haversine exceeding one", geo.Point{Latitude: big.NewRat(3, 2), Longitude: big.NewRat(0, 1)}, geo.Point{Latitude: big.NewRat(-3, 2), Longitude: big.NewRat(180, 1)}, r * math.Pi, 1},
		{"poles", point(-90, 0), point(90, 0), r * math.Pi, 1e-8},
		{"dateline", point(0, 179), point(0, -179), r * math.Pi / 90, 1e-8},
		{"unrounded rational", point(0, 0), geo.Point{Latitude: big.NewRat(0, 1), Longitude: big.NewRat(1, 600)}, r * math.Pi / 108000, 1e-8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := snapshot(t, []geo.Point{tt.a, tt.b})
			got, err := climate.Distance(tt.a, tt.b)
			if err != nil || math.IsNaN(got) || math.IsInf(got, 0) || got < 0 || got > r*math.Pi || math.Abs(got-tt.want) > tt.tolerance {
				t.Fatalf("distance=%.15g err=%v want=%.15g", got, err, tt.want)
			}
			reverse, err := climate.Distance(tt.b, tt.a)
			if err != nil || reverse != got || !bytes.Equal(before, snapshot(t, []geo.Point{tt.a, tt.b})) {
				t.Fatalf("reverse=%g err=%v or input changed", reverse, err)
			}
		})
	}
}

type candidateInput struct {
	origin    geo.Representative
	stations  []jma.StationNormals
	locations []climate.StationLocation
	element   jma.Element
}

func candidateFixture() candidateInput {
	p := point(0, 0)
	input := candidateInput{origin: geo.Representative{AreaCode: "99000", Point: &p, ElevationMeters: big.NewRat(201, 2)}, element: jma.ElementTemperature}
	for _, s := range []jma.Station{station("00003", 600, 0, 101), station("00002", 1, 0, 0), station("00001", -600, 0, 1000)} {
		input.stations = append(input.stations, jma.StationNormals{Station: s, Normals: []jma.Normal{normal(input.element, jma.StatusOK)}})
		code := "99000"
		if s.ID == "00002" {
			code = "99002"
		}
		input.locations = append(input.locations, climate.StationLocation{StationID: s.ID, Point: geo.Point{
			Latitude: big.NewRat(s.Latitude.TotalMinutesTenths, 600), Longitude: big.NewRat(s.Longitude.TotalMinutesTenths, 600),
		}, AreaCode: code})
	}
	return input
}

func inputSnapshot(t testing.TB, in candidateInput) []byte {
	t.Helper()
	return snapshot(t, []any{in.origin, in.stations, in.locations})
}

func TestCandidates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*candidateInput)
		ids    []string
	}{
		{"inside before outside and equal distance by ID without limits", func(*candidateInput) {}, []string{"00001", "00003", "00002"}},
		{"distance order within area", func(in *candidateInput) { in.locations[1].AreaCode = "99000" }, []string{"00002", "00001", "00003"}},
		{"distinct distances are not epsilon ties", func(in *candidateInput) { in.origin.Point.Latitude = big.NewRat(1, 10000000000000) }, []string{"00003", "00001", "00002"}},
		{"annual unavailable excluded", func(in *candidateInput) { in.stations[2].Normals[0].Annual = jma.Value{Status: jma.StatusUnavailable} }, []string{"00003", "00002"}},
		{"valid snow zero retained with missing months and observation mark", func(in *candidateInput) {
			in.element = jma.ElementMaximumSnowDepth
			for i := range in.stations {
				in.stations[i].Normals[0].Element = in.element
			}
		}, []string{"00001", "00003", "00002"}},
		{"missing elevation", func(in *candidateInput) { in.origin.ElevationMeters = nil }, []string{"00001", "00003", "00002"}},
		{"stale location unused", func(in *candidateInput) {
			in.locations = append(in.locations, climate.StationLocation{StationID: "99999"})
		}, []string{"00001", "00003", "00002"}},
		{"unrequested normal ignored", func(in *candidateInput) {
			in.stations[0].Normals = append(in.stations[0].Normals, jma.Normal{Element: jma.ElementDailyMaximum})
		}, []string{"00001", "00003", "00002"}},
		{"no candidates", func(in *candidateInput) {
			for i := range in.stations {
				in.stations[i].Normals[0].Annual = jma.Value{Status: jma.StatusUnavailable}
			}
		}, []string{}},
		{"empty input", func(in *candidateInput) { in.stations, in.locations = nil, nil }, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := candidateFixture()
			tt.change(&in)
			before := inputSnapshot(t, in)
			got, err := climate.Candidates(in.origin, in.stations, in.locations, in.element)
			if err != nil || got == nil {
				t.Fatalf("candidates=%v err=%v", got, err)
			}
			ids := make([]string, 0, len(got))
			for _, c := range got {
				ids = append(ids, c.StationID)
				for _, loc := range in.locations {
					if loc.StationID == c.StationID && c.Inside != (loc.AreaCode == in.origin.AreaCode) {
						t.Fatalf("inside=%t for station %s in area %s", c.Inside, c.StationID, loc.AreaCode)
					}
				}
				for _, sn := range in.stations {
					if sn.Station.ID != c.StationID {
						continue
					}
					wantDistance, err := climate.Distance(*in.origin.Point, geo.Point{Latitude: big.NewRat(sn.Station.Latitude.TotalMinutesTenths, 600), Longitude: big.NewRat(sn.Station.Longitude.TotalMinutesTenths, 600)})
					if err != nil || c.DistanceMeters != wantDistance {
						t.Fatalf("distance=%g want=%g err=%v", c.DistanceMeters, wantDistance, err)
					}
					if in.origin.ElevationMeters == nil {
						if c.ElevationDifferenceMeters != nil {
							t.Fatal("missing elevation must remain nil")
						}
					} else {
						want := new(big.Rat).Sub(big.NewRat(sn.Station.AltitudeMeters, 1), in.origin.ElevationMeters)
						if c.ElevationDifferenceMeters == nil || c.ElevationDifferenceMeters.Cmp(want) != 0 {
							t.Fatalf("elevation=%v want=%v", c.ElevationDifferenceMeters, want)
						}
					}
				}
			}
			if !slices.Equal(ids, tt.ids) || !bytes.Equal(before, inputSnapshot(t, in)) {
				t.Fatalf("ids=%v want=%v or input changed", ids, tt.ids)
			}
			slices.Reverse(in.stations)
			slices.Reverse(in.locations)
			reversed, err := climate.Candidates(in.origin, in.stations, in.locations, in.element)
			if err != nil || !reflect.DeepEqual(reversed, got) {
				t.Fatalf("input order affected candidates: %v", err)
			}
		})
	}
}

func TestElevationDifference(t *testing.T) {
	for _, tt := range []struct {
		name     string
		origin   *big.Rat
		altitude int64
		want     *big.Rat
	}{
		{"positive fraction", big.NewRat(201, 2), 101, big.NewRat(1, 2)},
		{"negative fraction", big.NewRat(201, 2), 0, big.NewRat(-201, 2)},
		{"zero", big.NewRat(100, 1), 100, big.NewRat(0, 1)},
		{"below sea level", big.NewRat(3, 2), -3, big.NewRat(-9, 2)},
		{"missing", nil, 100, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := candidateFixture()
			in.stations, in.locations = in.stations[:1], in.locations[:1]
			in.origin.ElevationMeters = tt.origin
			in.stations[0].Station.AltitudeMeters = tt.altitude
			before := inputSnapshot(t, in)
			got, err := climate.Candidates(in.origin, in.stations, in.locations, in.element)
			if err != nil || len(got) != 1 {
				t.Fatalf("candidates=%v err=%v want elevation=%v", got, err, tt.want)
			}
			difference := got[0].ElevationDifferenceMeters
			if (difference == nil) != (tt.want == nil) || (difference != nil && difference.Cmp(tt.want) != 0) {
				t.Fatalf("elevation=%v want=%v", difference, tt.want)
			}
			if got[0].ElevationDifferenceMeters != nil {
				got[0].ElevationDifferenceMeters.SetInt64(999)
			}
			if !bytes.Equal(before, inputSnapshot(t, in)) {
				t.Fatal("result aliases input elevation")
			}
		})
	}
}

func TestCandidatesByElement(t *testing.T) {
	in := candidateFixture()
	in.stations, in.locations = nil, nil
	for i, e := range elements {
		id := fmt.Sprintf("%05d", i+1)
		sn := jma.StationNormals{Station: station(id, 0, 0, 0)}
		for _, other := range elements {
			status := jma.StatusUnavailable
			if e == other {
				status = jma.StatusOK
			}
			sn.Normals = append(sn.Normals, normal(other, status))
		}
		in.stations = append(in.stations, sn)
		in.locations = append(in.locations, climate.StationLocation{StationID: id, Point: point(0, 0)})
	}
	before := inputSnapshot(t, in)
	for i, e := range elements {
		t.Run(string(e), func(t *testing.T) {
			got, err := climate.Candidates(in.origin, in.stations, in.locations, e)
			if err != nil || len(got) != 1 || got[0].StationID != fmt.Sprintf("%05d", i+1) {
				t.Fatalf("candidates=%v err=%v", got, err)
			}
		})
	}
	if !bytes.Equal(before, inputSnapshot(t, in)) {
		t.Fatal("input changed")
	}
}

func checkReadError(t testing.TB, err error, key, field string) {
	t.Helper()
	var where *climate.ReadError
	if !errors.Is(err, climate.ErrInvalidFormat) || !errors.As(err, &where) {
		t.Fatalf("error=%v, want ReadError wrapping ErrInvalidFormat", err)
	}
	if where.Key != key || where.Field != field || where.Source != "" || where.File != "" || where.Line != 0 {
		t.Fatalf("location=%+v, want key=%s field=%s with no file location", where, key, field)
	}
}

func TestLocateStationsErrors(t *testing.T) {
	b, overlapping := boundaries(t, false), boundaries(t, true)
	for _, tt := range []struct {
		name     string
		b        *geo.Boundaries
		stations []jma.Station
		key      string
		field    string
	}{
		{"nil boundaries", nil, nil, "", ""},
		{"short ID", b, []jma.Station{station("0001", 0, 0, 0)}, "0001", "StationID"},
		{"long ID", b, []jma.Station{station("000001", 0, 0, 0)}, "000001", "StationID"},
		{"non ASCII ID", b, []jma.Station{station("００００１", 0, 0, 0)}, "００００１", "StationID"},
		{"letter ID", b, []jma.Station{station("0000a", 0, 0, 0)}, "0000a", "StationID"},
		{"duplicate ID", b, []jma.Station{station("00001", 900, 900, 0), station("00001", 0, 0, 0)}, "00001", "StationID"},
		{"latitude range", b, []jma.Station{station("00001", 54001, 0, 0)}, "00001", "Latitude"},
		{"longitude range", b, []jma.Station{station("00001", 0, -108001, 0)}, "00001", "Longitude"},
		{"overlapping areas", overlapping, []jma.Station{station("00001", 900, 900, 0)}, "00001", "Point"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := snapshot(t, tt.stations)
			got, err := climate.LocateStations(tt.b, tt.stations)
			if got != nil || err == nil {
				t.Fatalf("locations=%v err=%v", got, err)
			}
			if tt.b == nil {
				if !errors.Is(err, climate.ErrInvalidArgument) {
					t.Fatal(err)
				}
			} else {
				checkReadError(t, err, tt.key, tt.field)
			}
			if tt.b == overlapping {
				var geoError *geo.ReadError
				if !errors.Is(err, geo.ErrInvalidFormat) || !errors.As(err, &geoError) {
					t.Fatalf("geo cause lost: %v", err)
				}
			}
			if !bytes.Equal(before, snapshot(t, tt.stations)) {
				t.Fatal("input changed on failure")
			}
		})
	}
}

func TestCandidatesErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*candidateInput)
		key    string
		field  string
	}{
		{"nil origin", func(in *candidateInput) { in.origin.Point = nil }, "", ""},
		{"nil origin latitude", func(in *candidateInput) { in.origin.Point.Latitude = nil }, "", ""},
		{"nil origin longitude", func(in *candidateInput) { in.origin.Point.Longitude = nil }, "", ""},
		{"out of range origin", func(in *candidateInput) { in.origin.Point.Longitude = big.NewRat(181, 1) }, "", ""},
		{"empty origin code", func(in *candidateInput) { in.origin.AreaCode = "" }, "", ""},
		{"invalid origin code", func(in *candidateInput) { in.origin.AreaCode = "bad" }, "", ""},
		{"empty element", func(in *candidateInput) { in.element = "" }, "", ""},
		{"unsupported element", func(in *candidateInput) { in.element = "2200" }, "", ""},
		{"invalid station ID", func(in *candidateInput) { in.stations[0].Station.ID = "x0003" }, "x0003", "StationID"},
		{"duplicate station ID", func(in *candidateInput) { in.stations = append(in.stations, in.stations[0]) }, "00003", "StationID"},
		{"invalid location ID", func(in *candidateInput) { in.locations[0].StationID = "0003" }, "0003", "StationID"},
		{"duplicate location ID", func(in *candidateInput) { in.locations = append(in.locations, in.locations[0]) }, "00003", "StationID"},
		{"duplicate stale ID", func(in *candidateInput) {
			in.locations = append(in.locations, climate.StationLocation{StationID: "99999"}, climate.StationLocation{StationID: "99999"})
		}, "99999", "StationID"},
		{"invalid location area", func(in *candidateInput) { in.locations[0].AreaCode = "bad" }, "00003", "AreaCode"},
		{"missing location", func(in *candidateInput) { in.locations = in.locations[1:] }, "00003", "StationID"},
		{"nil location latitude", func(in *candidateInput) { in.locations[0].Point.Latitude = nil }, "00003", "Point.Latitude"},
		{"nil location longitude", func(in *candidateInput) { in.locations[0].Point.Longitude = nil }, "00003", "Point.Longitude"},
		{"out of range location", func(in *candidateInput) { in.locations[0].Point.Longitude = big.NewRat(-181, 1) }, "00003", "Point.Longitude"},
		{"latitude mismatch", func(in *candidateInput) {
			in.locations[0].Point.Latitude.Add(in.locations[0].Point.Latitude, big.NewRat(1, 1000000000000000))
		}, "00003", "Point"},
		{"longitude mismatch", func(in *candidateInput) { in.locations[0].Point.Longitude = big.NewRat(1, 600) }, "00003", "Point"},
		{"station latitude range", func(in *candidateInput) { in.stations[0].Station.Latitude.TotalMinutesTenths = -54001 }, "00003", "Latitude"},
		{"station longitude range", func(in *candidateInput) { in.stations[0].Station.Longitude.TotalMinutesTenths = 108001 }, "00003", "Longitude"},
		{"missing normal", func(in *candidateInput) { in.stations[1].Normals = nil }, "00002", "Normals"},
		{"different normal", func(in *candidateInput) { in.stations[1].Normals[0].Element = jma.ElementDailyMaximum }, "00002", "Normals"},
		{"duplicate normal", func(in *candidateInput) {
			in.stations[1].Normals = append(in.stations[1].Normals, in.stations[1].Normals[0])
		}, "00002", "Normals"},
		{"unset annual", func(in *candidateInput) { in.stations[1].Normals[0].Annual.Status = jma.StatusUnset }, "00002", "Annual.Status"},
		{"unknown annual", func(in *candidateInput) { in.stations[1].Normals[0].Annual.Status = 99 }, "00002", "Annual.Status"},
		{"unset month", func(in *candidateInput) { in.stations[1].Normals[0].Monthly[0].Status = jma.StatusUnset }, "00002", "Monthly[0].Status"},
		{"unknown month", func(in *candidateInput) { in.stations[1].Normals[0].Monthly[11].Status = -1 }, "00002", "Monthly[11].Status"},
		{"unavailable still checked", func(in *candidateInput) {
			in.stations[0].Normals[0].Annual.Status = jma.StatusUnavailable
			in.locations[0].Point.Latitude = nil
		}, "00003", "Point.Latitude"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := candidateFixture()
			tt.change(&in)
			before := inputSnapshot(t, in)
			got, err := climate.Candidates(in.origin, in.stations, in.locations, in.element)
			if got != nil || err == nil {
				t.Fatalf("candidates=%v err=%v", got, err)
			}
			if tt.field == "" {
				if !errors.Is(err, climate.ErrInvalidArgument) {
					t.Fatal(err)
				}
			} else {
				checkReadError(t, err, tt.key, tt.field)
			}
			if !bytes.Equal(before, inputSnapshot(t, in)) {
				t.Fatal("input changed on failure")
			}
		})
	}
}

func TestDistanceErrors(t *testing.T) {
	for _, p := range []geo.Point{
		{}, {Latitude: big.NewRat(0, 1)}, {Longitude: big.NewRat(0, 1)},
		point(91, 0), point(-91, 0), point(0, 181), point(0, -181),
		{Latitude: big.NewRat(90000001, 1000000), Longitude: big.NewRat(0, 1)},
	} {
		t.Run(fmt.Sprint(p), func(t *testing.T) {
			for _, pair := range [][2]geo.Point{{p, point(0, 0)}, {point(0, 0), p}} {
				got, err := climate.Distance(pair[0], pair[1])
				if got != 0 || !errors.Is(err, climate.ErrInvalidArgument) {
					t.Fatalf("distance=%g err=%v", got, err)
				}
			}
		})
	}
}

func TestReadError(t *testing.T) {
	for _, tt := range []struct {
		name string
		at   climate.ReadError
		text []string
	}{
		{"memory", climate.ReadError{Key: "00001", Field: "Point", Err: climate.ErrInvalidFormat}, []string{"climate", "00001", "Point", "invalid format"}},
		{"file", climate.ReadError{Source: "source", File: "file.csv", Line: 2, Key: "00002", Field: "Annual.Status", Err: climate.ErrInvalidArgument}, []string{"climate", "source", "file.csv", "line 2", "00002", "Annual.Status", "invalid argument"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(&tt.at, tt.at.Err) || errors.Unwrap(&tt.at) != tt.at.Err {
				t.Fatal("cause lost")
			}
			for _, text := range tt.text {
				if !strings.Contains(tt.at.Error(), text) {
					t.Fatalf("error=%q, missing %q", tt.at.Error(), text)
				}
			}
		})
	}
}
