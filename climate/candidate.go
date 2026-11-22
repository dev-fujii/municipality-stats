package climate

import (
	"cmp"
	"fmt"
	"math/big"
	"slices"

	"github.com/dev-fujii/municipality-stats/geo"
	"github.com/dev-fujii/municipality-stats/jma"
)

// Candidate は代表点からの距離と、地点の標高から町の平均標高を引いた差を持つ。
type Candidate struct {
	StationID                 string
	Inside                    bool
	DistanceMeters            float64
	ElevationDifferenceMeters *big.Rat
}

// Candidates は年の値がある全地点を区域内、距離、地点 ID の順で返す。
// 標高の欠損は候補から除かず、上限の判定と例外の適用は呼び出す側に任せる。
func Candidates(origin geo.Representative, stations []jma.StationNormals, locations []StationLocation, element jma.Element) ([]Candidate, error) {
	if origin.Point == nil || !fiveDigits(origin.AreaCode) {
		return nil, fmt.Errorf("%w: nil origin Point or invalid AreaCode", ErrInvalidArgument)
	}
	if field := invalidPointField(*origin.Point); field != "" {
		return nil, fmt.Errorf("%w: nil or out-of-range origin %s", ErrInvalidArgument, field)
	}
	if !supportedElement(element) {
		return nil, fmt.Errorf("%w: unsupported element %q", ErrInvalidArgument, element)
	}
	byID, err := locationsByID(locations)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(stations))
	result := make([]Candidate, 0, len(stations))
	for _, sn := range stations {
		s := sn.Station
		where := ReadError{Key: s.ID}
		if err := checkStationID(s.ID, seen); err != nil {
			return nil, where.fail("StationID", err)
		}
		p := stationPoint(s)
		if field := invalidPointField(p); field != "" {
			return nil, where.fail(field, invalid("station coordinate out of range"))
		}
		loc, exists := byID[s.ID]
		if !exists {
			return nil, where.fail("StationID", invalid("station location missing"))
		}
		if field := invalidPointField(loc.Point); field != "" {
			return nil, where.fail("Point."+field, invalid("nil or out-of-range location coordinate"))
		}
		if loc.Point.Latitude.Cmp(p.Latitude) != 0 || loc.Point.Longitude.Cmp(p.Longitude) != 0 {
			return nil, where.fail("Point", invalid("location differs from station coordinates"))
		}
		ok, err := annualAvailable(sn.Normals, element, where)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		distance, err := Distance(*origin.Point, p)
		if err != nil {
			return nil, err
		}
		c := Candidate{StationID: s.ID, Inside: loc.AreaCode == origin.AreaCode, DistanceMeters: distance}
		if origin.ElevationMeters != nil {
			c.ElevationDifferenceMeters = new(big.Rat).Sub(big.NewRat(s.AltitudeMeters, 1), origin.ElevationMeters)
		}
		result = append(result, c)
	}
	slices.SortFunc(result, func(a, b Candidate) int {
		if a.Inside != b.Inside {
			if a.Inside {
				return -1
			}
			return 1
		}
		if order := cmp.Compare(a.DistanceMeters, b.DistanceMeters); order != 0 {
			return order
		}
		return cmp.Compare(a.StationID, b.StationID)
	})
	return result, nil
}

func locationsByID(locations []StationLocation) (map[string]StationLocation, error) {
	seen := make(map[string]bool, len(locations))
	result := make(map[string]StationLocation, len(locations))
	for _, loc := range locations {
		where := ReadError{Key: loc.StationID}
		if err := checkStationID(loc.StationID, seen); err != nil {
			return nil, where.fail("StationID", err)
		}
		if loc.AreaCode != "" && !fiveDigits(loc.AreaCode) {
			return nil, where.fail("AreaCode", invalid("area code must be empty or five ASCII digits"))
		}
		result[loc.StationID] = loc
	}
	return result, nil
}

func annualAvailable(normals []jma.Normal, element jma.Element, where ReadError) (bool, error) {
	var selected *jma.Normal
	for i := range normals {
		if normals[i].Element != element {
			continue
		}
		if selected != nil {
			return false, where.fail("Normals", invalid("duplicate normal for element %s", element))
		}
		selected = &normals[i]
	}
	if selected == nil {
		return false, where.fail("Normals", invalid("missing normal for element %s", element))
	}
	for i, v := range selected.Monthly {
		if v.Status != jma.StatusOK && v.Status != jma.StatusUnavailable {
			return false, where.fail(fmt.Sprintf("Monthly[%d].Status", i), invalid("invalid status for element %s", element))
		}
	}
	if selected.Annual.Status != jma.StatusOK && selected.Annual.Status != jma.StatusUnavailable {
		return false, where.fail("Annual.Status", invalid("invalid status for element %s", element))
	}
	return selected.Annual.Status == jma.StatusOK, nil
}

func supportedElement(e jma.Element) bool {
	switch e {
	case jma.ElementTemperature, jma.ElementDailyMaximum, jma.ElementDailyMinimum,
		jma.ElementSunshineDuration, jma.ElementMaximumSnowDepth, jma.ElementSnowfallTotal:
		return true
	default:
		return false
	}
}
