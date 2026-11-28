package rail

import "math"

// climate.Distance と同じ球半径（6,371,008.8m）で haversine の大円距離を km で返す。
const earthRadiusMeters = 6_371_008.8

func greatCircleDistanceKM(a, b Point) float64 {
	const radians = math.Pi / 180
	latA, lonA := a.Latitude*radians, a.Longitude*radians
	latB, lonB := b.Latitude*radians, b.Longitude*radians
	sinLat, sinLon := math.Sin((latB-latA)/2), math.Sin((lonB-lonA)/2)
	h := sinLat*sinLat + math.Cos(latA)*math.Cos(latB)*sinLon*sinLon
	h = min(1, max(0, h))
	return (2 * earthRadiusMeters * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))) / 1000
}
