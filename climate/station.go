// Package climate は geo と jma から要素ごとの観測地点を区域内優先で並べる。
package climate

import (
	"cmp"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/dev-fujii/municipality-stats/geo"
	"github.com/dev-fujii/municipality-stats/jma"
)

// StationLocation は地点の正確な座標と統合後の区域への所属を持つ。
type StationLocation struct {
	StationID  string
	Point      geo.Point
	AreaCode   string
	OnBoundary bool
}

// LocateStations は全国の地点を受け付け、地点 ID 順に所属を返す。
func LocateStations(b *geo.Boundaries, stations []jma.Station) ([]StationLocation, error) {
	if b == nil {
		return nil, fmt.Errorf("%w: nil Boundaries", ErrInvalidArgument)
	}
	seen := make(map[string]bool, len(stations))
	result := make([]StationLocation, 0, len(stations))
	for _, s := range stations {
		where := ReadError{Key: s.ID}
		if err := checkStationID(s.ID, seen); err != nil {
			return nil, where.fail("StationID", err)
		}
		p := stationPoint(s)
		if field := invalidPointField(p); field != "" {
			return nil, where.fail(field, invalid("station coordinate out of range"))
		}
		loc, err := b.Locate(p)
		if err != nil {
			return nil, where.fail("Point", errors.Join(ErrInvalidFormat, err))
		}
		result = append(result, StationLocation{StationID: s.ID, Point: p, AreaCode: loc.AreaCode, OnBoundary: loc.OnBoundary})
	}
	slices.SortFunc(result, func(a, b StationLocation) int { return cmp.Compare(a.StationID, b.StationID) })
	return result, nil
}

func stationPoint(s jma.Station) geo.Point {
	return geo.Point{
		Latitude:  big.NewRat(s.Latitude.TotalMinutesTenths, 600),
		Longitude: big.NewRat(s.Longitude.TotalMinutesTenths, 600),
	}
}

func fiveDigits(s string) bool {
	if len(s) != 5 {
		return false
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func checkStationID(id string, seen map[string]bool) error {
	if !fiveDigits(id) {
		return invalid("station ID must be five ASCII digits")
	}
	if seen[id] {
		return invalid("duplicate station ID")
	}
	seen[id] = true
	return nil
}
