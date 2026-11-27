package geo

import (
	"errors"
	"math"
	"math/big"
	"reflect"
	"slices"
	"testing"
)

func TestRepresentatives(t *testing.T) {
	b := readJSON(t, fixture(t, "rectangles.geojson"), nil)
	population := []PopulationCell{{"543900001", 1}, {"543900004", 3}}
	elevation := []ElevationCell{{"543900001", big.NewRat(100, 1)}, {"543900004", big.NewRat(300, 1)}}
	center := rationalPoint("6913/192", "17793/128")
	first := rationalPoint("17281/480", "44481/320")
	for _, tt := range []struct {
		name       string
		population []PopulationCell
		elevation  []ElevationCell
		people     string
		point      *Point
		meters     *big.Rat
		state      ElevationState
		missing    []string
	}{
		{"weighted mean", population, elevation, "4", &center, big.NewRat(250, 1), ElevationOK, nil},
		{"reversed population", []PopulationCell{population[1], population[0]}, elevation, "4", &center, big.NewRat(250, 1), ElevationOK, nil},
		{"reversed elevation", population, []ElevationCell{elevation[1], elevation[0]}, "4", &center, big.NewRat(250, 1), ElevationOK, nil},
		{"missing row", population, elevation[:1], "4", &center, nil, ElevationMissing, []string{"543900004"}},
		{"nil meters", population, []ElevationCell{elevation[0], {MeshCode: "543900004"}}, "4", &center, nil, ElevationMissing, []string{"543900004"}},
		{"sorted missing meshes", []PopulationCell{population[1], population[0]}, nil, "4", &center, nil, ElevationMissing, []string{"543900001", "543900004"}},
		{"zero population missing row", []PopulationCell{population[0], {"543900004", 0}}, elevation[:1], "1", &first, big.NewRat(100, 1), ElevationOK, nil},
		{"zero population nil meters", []PopulationCell{population[0], {"543900004", 0}}, []ElevationCell{elevation[0], {MeshCode: "543900004"}}, "1", &first, big.NewRat(100, 1), ElevationOK, nil},
		{"zero population", []PopulationCell{{"543900001", 0}}, nil, "0", nil, nil, ElevationNoPopulation, nil},
		{"no cells", nil, nil, "0", nil, nil, ElevationNoPopulation, nil},
		{"outside and unassigned", []PopulationCell{population[0], {"544600001", 999}, {"533900001", 999}}, elevation, "1", &first, big.NewRat(100, 1), ElevationOK, nil},
		{"zero and negative elevations", population, []ElevationCell{{"543900001", big.NewRat(0, 1)}, {"543900004", big.NewRat(-1, 3)}}, "4", &center, big.NewRat(-1, 4), ElevationOK, nil},
		{"population exceeds int64", []PopulationCell{{"543900001", math.MaxInt64}, {"543900004", math.MaxInt64}}, elevation, "18446744073709551614", pointPointer(rationalPoint("8641/240", "22241/160")), big.NewRat(200, 1), ElevationOK, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			codes := []string{"10002", "10001"}
			popBefore := slices.Clone(tt.population)
			heightsBefore := cloneElevationCells(tt.elevation)
			got, err := Representatives(b, tt.population, tt.elevation, codes)
			if err != nil || len(got) != 2 {
				t.Fatalf("got=%+v error=%v", got, err)
			}
			wantState := RepresentativeOK
			if tt.point == nil {
				wantState = RepresentativeNoPopulation
			}
			want := Representative{AreaCode: "10002", Point: tt.point, Population: integer(t, tt.people), ElevationMeters: tt.meters, State: wantState, ElevationState: tt.state, MissingElevationMeshes: tt.missing}
			if tt.point != nil {
				heights, _ := elevationByMesh(tt.elevation)
				covered, total := new(big.Int), new(big.Int)
				var cells, available int64
				for _, cell := range tt.population {
					if cell.People <= 0 {
						continue
					}
					center, _ := MeshCenter(cell.MeshCode)
					loc, _ := b.Locate(center)
					if loc.AreaCode != "10002" {
						continue
					}
					cells++
					total.Add(total, big.NewInt(cell.People))
					if heights[cell.MeshCode] != nil {
						available++
						covered.Add(covered, big.NewInt(cell.People))
					}
				}
				want.ElevationPopulationCoverage = new(big.Rat).SetFrac(covered, total)
				want.ElevationCellCoverage = big.NewRat(available, cells)
			}
			empty := Representative{AreaCode: "10001", Population: big.NewInt(0), State: RepresentativeNoPopulation, ElevationState: ElevationNoPopulation}
			if !reflect.DeepEqual(got, []Representative{empty, want}) {
				t.Fatalf("got=%+v, want=%+v", got, []Representative{empty, want})
			}
			if !reflect.DeepEqual(tt.population, popBefore) || !reflect.DeepEqual(tt.elevation, heightsBefore) || !slices.Equal(codes, []string{"10002", "10001"}) {
				t.Fatal("input changed")
			}
		})
	}
}

func integer(t testing.TB, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatal(s)
	}
	return n
}

func pointPointer(p Point) *Point { return &p }

func cloneElevationCells(cells []ElevationCell) []ElevationCell {
	result := slices.Clone(cells)
	for i := range result {
		if result[i].Meters != nil {
			result[i].Meters = new(big.Rat).Set(result[i].Meters)
		}
	}
	return result
}

func TestRepresentativesBoundary(t *testing.T) {
	b := readJSON(t, document(
		feature("10002", `[[[139.003125,36],[140,36],[140,37],[139.003125,37],[139.003125,36]]]`),
		feature("10001", `[[[139,36],[139.003125,36],[139.003125,37],[139,37],[139,36]]]`),
	), nil)
	center, err := MeshCenter("543900001")
	if err != nil {
		t.Fatal(err)
	}
	if location, err := b.Locate(center); err != nil || location != (Location{"10001", true}) {
		t.Fatalf("location=%+v error=%v", location, err)
	}
	for _, tt := range []struct {
		name  string
		codes []string
		want  []Representative
	}{
		{"all population in one area", []string{"10002", "10001"}, []Representative{
			{AreaCode: "10001", Point: &center, Population: big.NewInt(4), ElevationMeters: big.NewRat(100, 1), State: RepresentativeOK, ElevationState: ElevationOK, ElevationPopulationCoverage: big.NewRat(1, 1), ElevationCellCoverage: big.NewRat(1, 1)},
			{AreaCode: "10002", Population: big.NewInt(0), State: RepresentativeNoPopulation, ElevationState: ElevationNoPopulation},
		}},
		{"selected area does not receive neighbor population", []string{"10002"}, []Representative{{AreaCode: "10002", Population: big.NewInt(0), State: RepresentativeNoPopulation, ElevationState: ElevationNoPopulation}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Representatives(b, []PopulationCell{{"543900001", 4}}, []ElevationCell{{"543900001", big.NewRat(100, 1)}}, tt.codes)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got=%+v error=%v want=%+v", got, err, tt.want)
			}
		})
	}
}

func TestRepresentativesOutsideMean(t *testing.T) {
	b := readJSON(t, document(
		feature("10001", `[[[139,36],[139.004,36],[139.004,36.004],[139,36.004],[139,36]]]`),
		feature("10001", `[[[139.008,36.005],[139.011,36.005],[139.011,36.008],[139.008,36.008],[139.008,36.005]]]`),
	), nil)
	got, err := Representatives(b, []PopulationCell{{"543900001", 1}, {"543900004", 3}}, nil, []string{"10001"})
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0].Point, pointPointer(rationalPoint("6913/192", "17793/128"))) || got[0].State != RepresentativeOK {
		t.Fatalf("got=%+v error=%v", got, err)
	}
}

func TestRepresentativesInvalidInput(t *testing.T) {
	b := readJSON(t, fixture(t, "rectangles.geojson"), nil)
	for _, tt := range []struct {
		name       string
		boundaries *Boundaries
		codes      []string
		population []PopulationCell
		elevation  []ElevationCell
		key, field string
	}{
		{"nil boundaries", nil, []string{"10001"}, nil, nil, "", ""},
		{"nil codes", b, nil, nil, nil, "", ""},
		{"empty codes", b, []string{}, nil, nil, "", ""},
		{"empty code", b, []string{""}, nil, nil, "", ""},
		{"short code", b, []string{"1001"}, nil, nil, "", ""},
		{"non ASCII code", b, []string{"１０００１"}, nil, nil, "", ""},
		{"unknown code", b, []string{"99999"}, nil, nil, "99999", "AreaCode"},
		{"duplicate code", b, []string{"10001", "10001"}, nil, nil, "10001", "AreaCode"},
		{"population short mesh", b, []string{"10001"}, []PopulationCell{{"54390000", 0}}, nil, "54390000", "MeshCode"},
		{"population invalid quadrant", b, []string{"10001"}, []PopulationCell{{"543900000", 1}}, nil, "543900000", "MeshCode"},
		{"population invalid secondary", b, []string{"10001"}, []PopulationCell{{"543980001", 1}}, nil, "543980001", "MeshCode"},
		{"population duplicate zero", b, []string{"10001"}, []PopulationCell{{"543900001", 0}, {"543900001", 0}}, nil, "543900001", "MeshCode"},
		{"negative outside population", b, []string{"10001"}, []PopulationCell{{"533900001", -1}}, nil, "533900001", "People"},
		{"elevation invalid mesh", b, []string{"10001"}, nil, []ElevationCell{{MeshCode: "bad"}}, "bad", "MeshCode"},
		{"elevation duplicate nil", b, []string{"10001"}, nil, []ElevationCell{{MeshCode: "543900001"}, {"543900001", big.NewRat(1, 1)}}, "543900001", "MeshCode"},
		{"elevation duplicate value", b, []string{"10001"}, nil, []ElevationCell{{"543900001", big.NewRat(1, 1)}, {"543900001", big.NewRat(1, 1)}}, "543900001", "MeshCode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Representatives(tt.boundaries, tt.population, tt.elevation, tt.codes)
			if tt.field == "" {
				if got != nil || !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("got=%v error=%v", got, err)
				}
				return
			}
			var where *ReadError
			if got != nil || !errors.Is(err, ErrInvalidFormat) || !errors.As(err, &where) {
				t.Fatalf("got=%v error=%v", got, err)
			}
			if where.Key != tt.key || where.Field != tt.field || where.Source != "" || where.File != "" || where.Line != 0 {
				t.Fatalf("location=%+v", where)
			}
		})
	}
	t.Run("overlapping areas", func(t *testing.T) {
		overlap := readJSON(t, document(feature("10001", square), feature("10002", square)), nil)
		got, err := Representatives(overlap, []PopulationCell{{"543900001", 1}}, nil, []string{"10001"})
		if got != nil || !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("got=%v error=%v", got, err)
		}
	})
}

func TestRepresentativesOwnership(t *testing.T) {
	b := readJSON(t, fixture(t, "rectangles.geojson"), nil)
	shared := big.NewRat(100, 1)
	population := []PopulationCell{{"543900001", 1}, {"544000001", 1}}
	heights := []ElevationCell{{"543900001", shared}, {"544000001", shared}}
	codes := []string{"10002", "10001"}
	calculate := func() []Representative {
		t.Helper()
		r, err := Representatives(b, population, heights, codes)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	a, other := calculate(), calculate()
	a[0].Population.SetInt64(999)
	a[0].Point.Latitude.SetInt64(999)
	if a[0].Point.Longitude.Cmp(other[0].Point.Longitude) != 0 {
		t.Fatal("latitude and longitude share storage")
	}
	a[0].Point.Longitude.SetInt64(999)
	a[0].ElevationMeters.SetInt64(999)
	if !reflect.DeepEqual(other, calculate()) || !reflect.DeepEqual(a[1], other[1]) || shared.Cmp(big.NewRat(100, 1)) != 0 {
		t.Fatal("result mutation affected input or another result")
	}
	shared.SetInt64(500)
	population[0].People = 500
	heights[0].MeshCode = "changed"
	codes[0] = "changed"
	if other[1].Population.Int64() != 1 || other[1].ElevationMeters.Cmp(big.NewRat(100, 1)) != 0 {
		t.Fatal("input mutation affected result")
	}
	missing, err := Representatives(b, []PopulationCell{{"543900001", 1}, {"544000001", 1}}, nil, []string{"10001", "10002"})
	if err != nil {
		t.Fatal(err)
	}
	missing[0].MissingElevationMeshes[0] = "changed"
	if !slices.Equal(missing[1].MissingElevationMeshes, []string{"543900001"}) {
		t.Fatal("missing meshes share storage")
	}
	empty, err := Representatives(b, nil, nil, []string{"10001", "10002"})
	if err != nil {
		t.Fatal(err)
	}
	empty[0].Population.SetInt64(1)
	if empty[1].Population.Sign() != 0 {
		t.Fatal("zero populations share storage")
	}
}
