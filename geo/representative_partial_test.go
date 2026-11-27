package geo

import (
	"math/big"
	"reflect"
	"testing"
)

func TestRepresentativesPartialElevation(t *testing.T) {
	b := readJSON(t, fixture(t, "rectangles.geojson"), nil)
	population := []PopulationCell{{"543900001", 1}, {"543900002", 3}, {"543900004", 6}}
	elevation := []ElevationCell{{"543900001", big.NewRat(100, 1)}, {"543900002", big.NewRat(300, 1)}, {"543900004", big.NewRat(400, 1)}}
	for _, tc := range []struct {
		name                string
		count               int
		state               ElevationState
		mean, people, cells *big.Rat
	}{
		{"partial", 2, ElevationPartial, big.NewRat(250, 1), big.NewRat(2, 5), big.NewRat(2, 3)},
		{"missing", 0, ElevationMissing, nil, big.NewRat(0, 1), big.NewRat(0, 1)},
		{"complete", 3, ElevationOK, big.NewRat(340, 1), big.NewRat(1, 1), big.NewRat(1, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RepresentativesWithOptions(b, population, elevation[:tc.count], []string{"10002", "10001"}, RepresentativeOptions{AllowPartialElevation: true})
			if err != nil {
				t.Fatal(err)
			}
			r := got[1]
			if r.ElevationState != tc.state || !reflect.DeepEqual(r.ElevationMeters, tc.mean) || r.ElevationPopulationCoverage.Cmp(tc.people) != 0 || r.ElevationCellCoverage.Cmp(tc.cells) != 0 || len(r.MissingElevationMeshes) != 3-tc.count {
				t.Fatalf("%+v", r)
			}
			if got[0].ElevationPopulationCoverage != nil || got[0].ElevationCellCoverage != nil {
				t.Fatal(got[0])
			}
			original, err := Representatives(b, population, elevation[:tc.count], []string{"10002"})
			if err != nil {
				t.Fatal(err)
			}
			zero, err := RepresentativesWithOptions(b, population, elevation[:tc.count], []string{"10002"}, RepresentativeOptions{})
			if err != nil || !reflect.DeepEqual(original, zero) {
				t.Fatal("default behavior differs", err)
			}
			if tc.count < 3 && (original[0].ElevationState != ElevationMissing || original[0].ElevationMeters != nil) {
				t.Fatal(original)
			}
		})
	}
}
