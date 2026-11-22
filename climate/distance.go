package climate

import (
	"fmt"
	"math"
	"math/big"

	"github.com/dev-fujii/municipality-stats/geo"
)

const earthRadiusMeters = 6_371_008.8

// Distance は半径6,371,008.8mの球の大円距離を丸めずに返す。
func Distance(a, b geo.Point) (float64, error) {
	for _, p := range []geo.Point{a, b} {
		if field := invalidPointField(p); field != "" {
			return 0, fmt.Errorf("%w: nil or out-of-range %s", ErrInvalidArgument, field)
		}
	}
	latA, _ := a.Latitude.Float64()
	lonA, _ := a.Longitude.Float64()
	latB, _ := b.Latitude.Float64()
	lonB, _ := b.Longitude.Float64()
	radians := math.Pi / 180
	latA, lonA, latB, lonB = latA*radians, lonA*radians, latB*radians, lonB*radians
	sinLat, sinLon := math.Sin((latB-latA)/2), math.Sin((lonB-lonA)/2)
	h := sinLat*sinLat + math.Cos(latA)*math.Cos(latB)*sinLon*sinLon
	h = min(1, max(0, h))
	return 2 * earthRadiusMeters * math.Atan2(math.Sqrt(h), math.Sqrt(1-h)), nil
}

func invalidPointField(p geo.Point) string {
	for _, c := range []struct {
		name  string
		value *big.Rat
		limit int64
	}{{"Latitude", p.Latitude, 90}, {"Longitude", p.Longitude, 180}} {
		if c.value == nil || c.value.Cmp(big.NewRat(-c.limit, 1)) < 0 || c.value.Cmp(big.NewRat(c.limit, 1)) > 0 {
			return c.name
		}
	}
	return ""
}
