package rail

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// ExtraStation は事業者種別とは別に追加で選ぶ駅を表す。
type ExtraStation struct {
	LineName        string
	Operator        string
	StationName     string
	DisplayLineName string
}

// Station はグループコードで1つにまとめた駅を表す。
type Station struct {
	GroupCode string
	Name      string
	Lines     []string
	Positions []Point
}

// NearestResult は最も近い駅と代表点からの大円距離（km）を表す。
type NearestResult struct {
	Station    Station
	DistanceKM float64
}

// Midpoint は経度・緯度の平面上で線の長さの半分に当たる点を返す。
func Midpoint(coords []Point) (Point, error) {
	if len(coords) < 2 {
		return Point{}, fmt.Errorf("%w: LineString requires at least 2 points", ErrInvalidArgument)
	}
	for i, p := range coords {
		if !validPoint(p) {
			return Point{}, fmt.Errorf("%w: coordinate[%d] out of range", ErrInvalidArgument, i)
		}
	}

	lengths := make([]float64, len(coords)-1)
	var total float64
	for i := range len(coords) - 1 {
		d := math.Hypot(coords[i+1].Longitude-coords[i].Longitude, coords[i+1].Latitude-coords[i].Latitude)
		lengths[i] = d
		total += d
	}
	if total == 0 {
		return coords[0], nil
	}

	half := total / 2
	var acc float64
	for i, d := range lengths {
		if d > 0 && acc+d >= half {
			t := min(1, max(0, (half-acc)/d))
			return Point{
				Latitude:  coords[i].Latitude + t*(coords[i+1].Latitude-coords[i].Latitude),
				Longitude: coords[i].Longitude + t*(coords[i+1].Longitude-coords[i].Longitude),
			}, nil
		}
		acc += d
	}
	return coords[len(coords)-1], nil
}

type extraKey struct {
	lineName    string
	operator    string
	stationName string
}

type posEntry struct {
	stationCode string
	lineName    string
	operator    string
	pos         Point
}

type stationGroup struct {
	groupCode string
	name      string
	lines     map[string]bool
	entries   []posEntry
}

// Select は事業者種別と追加の駅の指定に一致するレコードを選び、グループコードごとに1駅へまとめる。
// 結果は GroupCode 昇順で返し、入力のスライスは変更しない。
func Select(records []Record, institutionTypes []string, extras []ExtraStation) ([]Station, error) {
	typeSet, err := validateInstitutionTypes(institutionTypes)
	if err != nil {
		return nil, err
	}
	extraIndex, err := validateExtras(extras)
	if err != nil {
		return nil, err
	}

	extraFound := make([]bool, len(extras))
	groups := make(map[string]*stationGroup)

	for i, r := range records {
		base := fmt.Sprintf("Records[%d]", i)
		if err := validateRecordFields(r, base); err != nil {
			return nil, err
		}

		matchedType := typeSet[r.InstitutionType]
		extraIdx, matchedExtra := extraIndex[extraKey{
			lineName:    r.LineName,
			operator:    r.Operator,
			stationName: r.StationName,
		}]
		if !matchedType && !matchedExtra {
			continue
		}

		pos, err := Midpoint(r.Coordinates)
		if err != nil {
			where := ReadError{Key: r.GroupCode}
			return nil, where.fail(base+".Coordinates", errors.Join(ErrInvalidFormat, err))
		}

		g, ok := groups[r.GroupCode]
		if !ok {
			g = &stationGroup{
				groupCode: r.GroupCode,
				name:      r.StationName,
				lines:     make(map[string]bool),
			}
			groups[r.GroupCode] = g
		} else if g.name != r.StationName {
			where := ReadError{Key: r.GroupCode}
			return nil, where.fail(base+".StationName", invalid("conflicting station names %q and %q in group %s", g.name, r.StationName, r.GroupCode))
		}

		line := r.LineName
		if matchedExtra {
			extraFound[extraIdx] = true
			// 在来線として載る駅は、その線の名前ではなく呼び出す側が渡した表示用の路線名で数える。
			if display := extras[extraIdx].DisplayLineName; display != "" && !matchedType {
				line = display
			}
		}
		g.lines[line] = true
		g.entries = append(g.entries, posEntry{
			stationCode: r.StationCode,
			lineName:    r.LineName,
			operator:    r.Operator,
			pos:         pos,
		})
	}

	for i, found := range extraFound {
		if !found {
			e := extras[i]
			key := fmt.Sprintf("%s/%s/%s", e.LineName, e.Operator, e.StationName)
			where := ReadError{Key: key}
			return nil, where.fail(fmt.Sprintf("Extras[%d]", i), fmt.Errorf("%w: %s", ErrExtraNotFound, key))
		}
	}

	result := make([]Station, 0, len(groups))
	for _, g := range groups {
		lines := make([]string, 0, len(g.lines))
		for line := range g.lines {
			lines = append(lines, line)
		}
		slices.Sort(lines)

		slices.SortFunc(g.entries, func(a, b posEntry) int {
			return cmp.Or(
				cmp.Compare(a.stationCode, b.stationCode),
				cmp.Compare(a.lineName, b.lineName),
				cmp.Compare(a.operator, b.operator),
				cmp.Compare(a.pos.Latitude, b.pos.Latitude),
				cmp.Compare(a.pos.Longitude, b.pos.Longitude),
			)
		})
		positions := make([]Point, len(g.entries))
		for i, e := range g.entries {
			positions[i] = e.pos
		}
		result = append(result, Station{
			GroupCode: g.groupCode,
			Name:      g.name,
			Lines:     lines,
			Positions: positions,
		})
	}
	slices.SortFunc(result, func(a, b Station) int {
		return cmp.Compare(a.GroupCode, b.GroupCode)
	})
	return result, nil
}

// Nearest は代表点から最も近い駅と大円距離（km）を返す。
// 距離は駅の持つ位置のうち最も近いものとし、同じ距離なら GroupCode の小さい方を採る。
func Nearest(stations []Station, point Point) (NearestResult, error) {
	if len(stations) == 0 {
		return NearestResult{}, fmt.Errorf("%w: stations must not be empty", ErrInvalidArgument)
	}
	if !validPoint(point) {
		return NearestResult{}, fmt.Errorf("%w: point out of range", ErrInvalidArgument)
	}

	seen := make(map[string]bool, len(stations))
	var bestStation Station
	var bestDist float64
	hasBest := false

	for i, s := range stations {
		if s.GroupCode == "" || s.Name == "" {
			return NearestResult{}, fmt.Errorf("%w: stations[%d] requires non-empty GroupCode and Name", ErrInvalidArgument, i)
		}
		if seen[s.GroupCode] {
			return NearestResult{}, fmt.Errorf("%w: duplicate station GroupCode %q", ErrInvalidArgument, s.GroupCode)
		}
		seen[s.GroupCode] = true
		if len(s.Positions) == 0 {
			return NearestResult{}, fmt.Errorf("%w: stations[%d] (%s) has no positions", ErrInvalidArgument, i, s.GroupCode)
		}

		var minDist float64
		for j, pos := range s.Positions {
			if !validPoint(pos) {
				return NearestResult{}, fmt.Errorf("%w: stations[%d].Positions[%d] out of range", ErrInvalidArgument, i, j)
			}
			d := greatCircleDistanceKM(point, pos)
			if j == 0 || d < minDist {
				minDist = d
			}
		}

		if !hasBest || minDist < bestDist || (minDist == bestDist && s.GroupCode < bestStation.GroupCode) {
			bestStation = s
			bestDist = minDist
			hasBest = true
		}
	}

	return NearestResult{
		Station: Station{
			GroupCode: bestStation.GroupCode,
			Name:      bestStation.Name,
			Lines:     slices.Clone(bestStation.Lines),
			Positions: slices.Clone(bestStation.Positions),
		},
		DistanceKM: bestDist,
	}, nil
}

func validateInstitutionTypes(types []string) (map[string]bool, error) {
	set := make(map[string]bool, len(types))
	for _, t := range types {
		if t == "" || strings.TrimSpace(t) != t {
			return nil, fmt.Errorf("%w: institution type must be non-empty without surrounding whitespace", ErrInvalidArgument)
		}
		if set[t] {
			return nil, fmt.Errorf("%w: duplicate institution type %q", ErrInvalidArgument, t)
		}
		set[t] = true
	}
	return set, nil
}

func validateExtras(extras []ExtraStation) (map[extraKey]int, error) {
	index := make(map[extraKey]int, len(extras))
	for i, e := range extras {
		if e.LineName == "" || strings.TrimSpace(e.LineName) != e.LineName ||
			e.Operator == "" || strings.TrimSpace(e.Operator) != e.Operator ||
			e.StationName == "" || strings.TrimSpace(e.StationName) != e.StationName {
			return nil, fmt.Errorf("%w: extras[%d] requires non-empty LineName, Operator and StationName without surrounding whitespace", ErrInvalidArgument, i)
		}
		if strings.TrimSpace(e.DisplayLineName) != e.DisplayLineName {
			return nil, fmt.Errorf("%w: extras[%d].DisplayLineName must not have surrounding whitespace", ErrInvalidArgument, i)
		}
		k := extraKey{
			lineName:    e.LineName,
			operator:    e.Operator,
			stationName: e.StationName,
		}
		if _, dup := index[k]; dup {
			return nil, fmt.Errorf("%w: duplicate extra station %s/%s/%s", ErrInvalidArgument, e.LineName, e.Operator, e.StationName)
		}
		index[k] = i
	}
	return index, nil
}

func validateRecordFields(r Record, base string) error {
	where := ReadError{Key: r.GroupCode}
	switch {
	case r.GroupCode == "":
		return where.fail(base+".GroupCode", invalid("GroupCode must not be empty"))
	case r.InstitutionType == "":
		return where.fail(base+".InstitutionType", invalid("InstitutionType must not be empty"))
	case r.LineName == "":
		return where.fail(base+".LineName", invalid("LineName must not be empty"))
	case r.Operator == "":
		return where.fail(base+".Operator", invalid("Operator must not be empty"))
	case r.StationName == "":
		return where.fail(base+".StationName", invalid("StationName must not be empty"))
	case r.StationCode == "":
		return where.fail(base+".StationCode", invalid("StationCode must not be empty"))
	}
	return nil
}
