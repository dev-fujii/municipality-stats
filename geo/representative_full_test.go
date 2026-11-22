package geo

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var representativeFullCodes = []string{"10204", "10205", "10464"}

func representativeFullInputs(t testing.TB) (*Boundaries, []PopulationCell, []ElevationCell) {
	t.Helper()
	dir := os.Getenv("GEO_TEST_ZIP_DIR")
	if dir == "" {
		t.Skip("GEO_TEST_ZIP_DIR is not set")
	}
	sources := make([]ZIPSource, 3)
	for i, name := range []string{"N03-20260101_10_GML.zip", "tblT001141H5439.zip", "G04-c-11_5439-jgd_GML.zip"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		sources[i] = ZIPSource{Name: name, Reader: bytes.NewReader(data), Size: int64(len(data))}
		var raw []byte
		switch i {
		case 0:
			raw, _, err = n03Bytes(sources[i])
		case 1:
			raw, _, err = selectedZIPBytes(sources[i], populationPath, "population text")
		case 2:
			raw, _, err = selectedZIPBytes(sources[i], elevationPath, "G04-c XML")
		}
		if err != nil {
			t.Fatal(err)
		}
		zipHash, contentHash := fmt.Sprintf("%x", sha256.Sum256(data)), fmt.Sprintf("%x", sha256.Sum256(raw))
		if (i == 0 && zipHash != "0981c3412b098ff2a9947e7a8bc57bc353275c2c337b76f7adea52c08b0d7b07") ||
			(i == 1 && contentHash != "32d3bb761e2dd13c0be08a850ee9f7820c3f4eb7b4f2ca11f81a17de0b93cd5a") ||
			(i == 2 && contentHash != "e7dbfd8607d12e72a0ef49b188cda0dd365174d69d8b0ae0fdcc573c741b98df") {
			t.Fatalf("%s ZIP_SHA256=%s content_sha256=%s; compare the contents and testdata/README.md", name, zipHash, contentHash)
		}
		t.Logf("%s ZIP_SHA256=%s content_sha256=%s", name, zipHash, contentHash)
	}
	b, err := ReadN03(sources[:1], nil)
	if err != nil {
		t.Fatal(err)
	}
	population, err := ReadPopulation(sources[1:2])
	if err != nil {
		t.Fatal(err)
	}
	elevation, err := ReadElevation(sources[2:])
	if err != nil {
		t.Fatal(err)
	}
	return b, population, elevation
}

func TestRepresentativesFullZIP(t *testing.T) {
	b, population, elevation := representativeFullInputs(t)
	start := time.Now()
	representatives, err := Representatives(b, population, elevation, representativeFullCodes)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(representatives) != len(representativeFullCodes) {
		t.Fatalf("representatives=%d", len(representatives))
	}
	t.Logf("population_cells=%d elevation_cells=%d areas=%d Representatives=%s (excludes ZIP reads and hashing)", len(population), len(elevation), len(representatives), elapsed)
	for i, tt := range []struct{ code, name string }{{"10204", "伊勢崎市"}, {"10205", "太田市"}, {"10464", "玉村町"}} {
		t.Run(tt.name, func(t *testing.T) {
			r := representatives[i]
			if r.AreaCode != tt.code || r.State != RepresentativeOK || r.ElevationState != ElevationOK || r.Population == nil || r.Population.Sign() <= 0 || r.Point == nil || r.ElevationMeters == nil || len(r.MissingElevationMeshes) != 0 {
				t.Fatalf("representative=%+v", r)
			}
			var selected *area
			for j := range b.areas {
				if b.areas[j].code == tt.code {
					selected = &b.areas[j]
				}
			}
			if selected == nil {
				t.Fatal("area not found")
			}
			mesh := rectangle{139 * coordinateScale, 36 * coordinateScale, 140 * coordinateScale, 110 * decimalScale}
			if !mesh.contains(intPoint{selected.box.minX, selected.box.minY}) || !mesh.contains(intPoint{selected.box.maxX, selected.box.maxY}) {
				t.Fatal("area extends beyond primary mesh 5439")
			}
			inside := representativeInside(*selected, *r.Point)
			if !inside || r.ElevationMeters.Sign() < 0 || r.ElevationMeters.Cmp(big.NewRat(300, 1)) > 0 {
				t.Fatalf("inside=%v representative=%+v", inside, r)
			}
			t.Logf("code=%s population=%s latitude=%s longitude=%s elevation_m=%s inside=%v wholly_in_5439=true", tt.code, r.Population, r.Point.Latitude.FloatString(9), r.Point.Longitude.FloatString(9), r.ElevationMeters.FloatString(6), inside)
		})
	}
}

func BenchmarkRepresentativesFullZIP(b *testing.B) {
	boundaries, population, elevation := representativeFullInputs(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := Representatives(boundaries, population, elevation, representativeFullCodes); err != nil {
			b.Fatal(err)
		}
	}
}

func representativeInside(a area, p Point) bool {
	// 加重平均の分母は Locate の固定精度に収まるとは限らないため、有理数のまま照合する。
	x := new(big.Rat).Mul(p.Longitude, new(big.Rat).SetInt64(coordinateScale))
	y := new(big.Rat).Mul(p.Latitude, new(big.Rat).SetInt64(coordinateScale))
	for _, poly := range a.polygons {
		inside, boundary := representativeRingLocation(poly.rings[0], x, y)
		if boundary {
			return true
		}
		if !inside {
			continue
		}
		inHole := false
		for _, hole := range poly.rings[1:] {
			in, on := representativeRingLocation(hole, x, y)
			if on {
				return true
			}
			inHole = inHole || in
		}
		if !inHole {
			return true
		}
	}
	return false
}

func representativeRingLocation(r ring, x, y *big.Rat) (inside, boundary bool) {
	previous := r.points[len(r.points)-1]
	for _, next := range r.points {
		ay, by := new(big.Rat).SetInt64(previous.y), new(big.Rat).SetInt64(next.y)
		if y.Cmp(new(big.Rat).SetInt64(min(previous.y, next.y))) >= 0 && y.Cmp(new(big.Rat).SetInt64(max(previous.y, next.y))) <= 0 {
			var dx, dy, py, px, cross, term big.Rat
			dx.SetInt64(next.x - previous.x)
			dy.SetInt64(next.y - previous.y)
			py.Sub(y, ay)
			px.Sub(x, new(big.Rat).SetInt64(previous.x))
			cross.Sub(term.Mul(&dx, &py), new(big.Rat).Mul(&dy, &px))
			if cross.Sign() == 0 && x.Cmp(new(big.Rat).SetInt64(min(previous.x, next.x))) >= 0 && x.Cmp(new(big.Rat).SetInt64(max(previous.x, next.x))) <= 0 {
				return false, true
			}
			if (ay.Cmp(y) > 0) != (by.Cmp(y) > 0) && ((next.y > previous.y && cross.Sign() > 0) || (next.y < previous.y && cross.Sign() < 0)) {
				inside = !inside
			}
		}
		previous = next
	}
	return inside, false
}
