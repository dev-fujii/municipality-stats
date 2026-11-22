package geo

import (
	"errors"
	"math/big"
	"slices"
	"testing"
)

func TestMeshCenter(t *testing.T) {
	b := readJSON(t, document(feature("10001", `[[[100,0],[180,0],[180,90],[100,90],[100,0]]]`)), nil)
	for _, tt := range []struct {
		code                string
		latitude, longitude string
	}{
		{"543900001", "17281/480", "44481/320"},
		{"543900002", "17281/480", "44483/320"},
		{"543900003", "17283/480", "44481/320"},
		{"543900004", "17283/480", "44483/320"},
		{"543900013", "17283/480", "44485/320"},
		{"543977994", "17599/480", "44799/320"},
		{"554000001", "17601/480", "44801/320"},
		{"000000001", "1/480", "32001/320"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			point, err := MeshCenter(tt.code)
			want := rationalPoint(tt.latitude, tt.longitude)
			if err != nil || point.Latitude.Cmp(want.Latitude) != 0 || point.Longitude.Cmp(want.Longitude) != 0 {
				t.Fatalf("point=%+v error=%v want=%+v", point, err, want)
			}
			for _, coordinate := range []*big.Rat{point.Latitude, point.Longitude} {
				scaled := new(big.Rat).Mul(coordinate, big.NewRat(coordinateScale, 1))
				if !scaled.IsInt() {
					t.Fatalf("coordinate cannot be represented exactly: %s", coordinate)
				}
			}
			location, err := b.Locate(point)
			if err != nil || location != (Location{AreaCode: "10001"}) {
				t.Fatalf("Locate=%+v error=%v", location, err)
			}
			point.Latitude.SetInt64(0)
			if point.Longitude.Cmp(want.Longitude) != 0 {
				t.Fatal("coordinates share storage")
			}
			again, err := MeshCenter(tt.code)
			if err != nil || again.Latitude.Cmp(want.Latitude) != 0 {
				t.Fatal("changing one result affects another")
			}
		})
	}
}

func TestMeshCenterInvalidCode(t *testing.T) {
	for _, code := range []string{
		"", "54390001", "5439000011", "54390000X", "５43900001", "5439000X1", "54390X001",
		"X43900001", "543980001", "543909001", "543900000", "543900005", "54390000-",
	} {
		t.Run(code, func(t *testing.T) {
			point, err := MeshCenter(code)
			if point.Latitude != nil || point.Longitude != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("point=%+v error=%v", point, err)
			}
		})
	}
}

func TestPrimaryMeshes(t *testing.T) {
	for _, tt := range []struct {
		name, coordinates string
		want              []string
	}{
		{"one mesh", `[[[139.1,36.1],[139.9,36.1],[139.9,36.5],[139.1,36.5],[139.1,36.1]]]`, []string{"5439"}},
		{"frame contact", `[[[139,36],[140,36],[140,36.5],[139,36.5],[139,36]]]`, []string{"5439"}},
		{"latitude frame contact", `[[[139,36],[140,36],[140,38],[139,38],[139,36]]]`, []string{"5439", "5539", "5639"}},
		{"crosses longitude", `[[[139.5,36.1],[140.5,36.1],[140.5,36.5],[139.5,36.5],[139.5,36.1]]]`, []string{"5439", "5440"}},
		{"hole removes meshes", `[[[139,35],[142,35],[142,39],[139,39],[139,35]],[[140,36],[141,36],[141,38],[140,38],[140,36]]]`, []string{"5239", "5240", "5241", "5339", "5340", "5341", "5439", "5441", "5539", "5541", "5639", "5641", "5739", "5740", "5741", "5839", "5840", "5841"}},
		{"triangle excludes bbox corner", `[[[139,36],[141,36],[139,38],[139,36]]]`, []string{"5439", "5440", "5539", "5540", "5639"}},
		{"thin horizontal strip", `[[[138,36.1],[142,36.1],[142,36.2],[138,36.2],[138,36.1]]]`, []string{"5438", "5439", "5440", "5441"}},
		{"negative coordinates", `[[[-140,-37],[-139,-37],[-139,-36],[-140,-36],[-140,-37]]]`, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := readJSON(t, document(feature("10001", tt.coordinates)), nil)
			if got := b.PrimaryMeshes(); !slices.Equal(got, tt.want) {
				t.Fatalf("got=%v want=%v", got, tt.want)
			}
		})
	}
	b := readJSON(t, document(feature("10001", square), feature("10001", square), feature("10002", square)), nil)
	if got := b.PrimaryMeshes(); !slices.Equal(got, []string{"5439", "5539"}) {
		t.Fatal(got)
	}
}

func TestSegmentIntersectsOpenRectangle(t *testing.T) {
	r := rectangle{0, 0, 10, 10}
	for _, tt := range []struct {
		name string
		a, b intPoint
		want bool
	}{
		{"horizontal", intPoint{-5, 5}, intPoint{15, 5}, true},
		{"vertical", intPoint{5, -5}, intPoint{5, 15}, true},
		{"diagonal corners", intPoint{-5, -5}, intPoint{15, 15}, true},
		{"inside", intPoint{2, 3}, intPoint{4, 5}, true},
		{"enters corner", intPoint{0, 0}, intPoint{5, 5}, true},
		{"touch corner", intPoint{-5, 5}, intPoint{5, -5}, false},
		{"touch end", intPoint{-5, -5}, intPoint{0, 0}, false},
		{"horizontal frame", intPoint{-5, 0}, intPoint{15, 0}, false},
		{"vertical frame", intPoint{0, -5}, intPoint{0, 15}, false},
		{"outside", intPoint{11, 2}, intPoint{15, 4}, false},
		{"zero length", intPoint{5, 5}, intPoint{5, 5}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := segmentIntersectsOpenRectangle(tt.a, tt.b, r); got != tt.want {
				t.Fatalf("got=%v want=%v", got, tt.want)
			}
			if got := segmentIntersectsOpenRectangle(tt.b, tt.a, r); got != tt.want {
				t.Fatalf("reversed got=%v want=%v", got, tt.want)
			}
		})
	}
}
