package climate_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dev-fujii/municipality-stats/climate"
	"github.com/dev-fujii/municipality-stats/geo"
	"github.com/dev-fujii/municipality-stats/jma"
)

func fullZIP(t testing.TB, dir, name, wantZIP, contentName, wantContent, wantCSVSet string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	zipHash := fmt.Sprintf("%x", sha256.Sum256(data))
	if wantZIP != "" && zipHash != wantZIP {
		t.Fatalf("%s ZIP_SHA256=%s; compare the contents and testdata/README.md", name, zipHash)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(z.File, func(a, b *zip.File) int { return strings.Compare(a.Name, b.Name) })
	setHash := sha256.New()
	contentHash := ""
	for _, f := range z.File {
		if f.FileInfo().IsDir() || (f.Name != contentName && (wantCSVSet == "" || !strings.HasSuffix(f.Name, ".csv"))) {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(r)
		closeErr := r.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("%s: read=%v close=%v", f.Name, readErr, closeErr)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(content))
		if f.Name == contentName {
			contentHash = hash
		}
		if wantCSVSet != "" {
			if _, err := fmt.Fprintf(setHash, "%s\x00%s\n", f.Name, hash); err != nil {
				t.Fatal(err)
			}
		}
	}
	if wantContent != "" && contentHash != wantContent {
		t.Fatalf("%s %s SHA256=%s; compare testdata/README.md", name, contentName, contentHash)
	}
	if wantCSVSet != "" && fmt.Sprintf("%x", setHash.Sum(nil)) != wantCSVSet {
		t.Fatalf("%s CSV set SHA256=%x; compare testdata/README.md", name, setHash.Sum(nil))
	}
	t.Logf("%s ZIP_SHA256=%s", name, zipHash)
	if wantContent != "" {
		t.Logf("%s content_sha256=%s", contentName, contentHash)
	}
	if wantCSVSet != "" {
		t.Logf("%s CSV_set_sha256=%x", name, setHash.Sum(nil))
	}
	return data
}

func TestCandidatesFullZIP(t *testing.T) {
	dir := os.Getenv("GEO_TEST_ZIP_DIR")
	if dir == "" {
		t.Skip("GEO_TEST_ZIP_DIR is not set")
	}
	start := time.Now()
	n03 := fullZIP(t, dir, "N03-20260101_10_GML.zip", "0981c3412b098ff2a9947e7a8bc57bc353275c2c337b76f7adea52c08b0d7b07", "", "", "")
	populationZIP := fullZIP(t, dir, "tblT001141H5439.zip", "", "tblT001141H5439.txt", "32d3bb761e2dd13c0be08a850ee9f7820c3f4eb7b4f2ca11f81a17de0b93cd5a", "")
	elevationZIP := fullZIP(t, dir, "G04-c-11_5439-jgd_GML.zip", "", "G04-c-11_5439-jgd.xml", "e7dbfd8607d12e72a0ef49b188cda0dd365174d69d8b0ae0fdcc573c741b98df", "")
	monthly := fullZIP(t, dir, "normal_amedas_monthly.zip", "", "", "", "003f6db30608b088b34b29c49ae05628ecf1a1dd0ed01329d940a0bccd5fd6ad")
	index := fullZIP(t, dir, "amedas_station_index.zip", "", "amedas_station_index.csv", "ce78300364c905ffbc68462fa790afddbbb04ac6f36b181e8efdfd6d5e2abc67", "")
	geoSource := func(name string, data []byte) geo.ZIPSource {
		return geo.ZIPSource{Name: name, Reader: bytes.NewReader(data), Size: int64(len(data))}
	}
	b, err := geo.ReadN03([]geo.ZIPSource{geoSource("N03-20260101_10_GML.zip", n03)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	population, err := geo.ReadPopulation([]geo.ZIPSource{geoSource("tblT001141H5439.zip", populationZIP)})
	if err != nil {
		t.Fatal(err)
	}
	elevation, err := geo.ReadElevation([]geo.ZIPSource{geoSource("G04-c-11_5439-jgd_GML.zip", elevationZIP)})
	if err != nil {
		t.Fatal(err)
	}
	dataset, err := jma.Read(jma.ZIPSource{Reader: bytes.NewReader(monthly), Size: int64(len(monthly))}, jma.ZIPSource{Reader: bytes.NewReader(index), Size: int64(len(index))}, elements)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read and hash inputs=%s", time.Since(start))
	stations := make([]jma.Station, 0, len(dataset.Stations))
	for _, sn := range dataset.Stations {
		stations = append(stations, sn.Station)
	}
	start = time.Now()
	locations, err := climate.LocateStations(b, stations)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LocateStations stations=%d elapsed=%s", len(locations), time.Since(start))
	start = time.Now()
	representatives, err := geo.Representatives(b, population, elevation, []string{"10204", "10205", "10464"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Representatives=%s", time.Since(start))
	for _, origin := range representatives {
		t.Run(origin.AreaCode, func(t *testing.T) {
			if origin.Point == nil || origin.ElevationMeters == nil {
				t.Fatalf("incomplete representative: %+v", origin)
			}
			for _, element := range elements {
				start := time.Now()
				candidates, err := climate.Candidates(origin, dataset.Stations, locations, element)
				if err != nil || len(candidates) == 0 {
					t.Fatalf("element=%s candidates=%d err=%v", element, len(candidates), err)
				}
				c := candidates[0]
				t.Logf("element=%s station=%s inside=%t distance_m=%.12f elevation_difference_m=%s (%s) candidates=%d elapsed=%s",
					element, c.StationID, c.Inside, c.DistanceMeters, c.ElevationDifferenceMeters.FloatString(12), c.ElevationDifferenceMeters.RatString(), len(candidates), time.Since(start))
			}
		})
	}
}
