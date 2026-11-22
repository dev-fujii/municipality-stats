// Package geo は行政区域を読み、正確な座標で所属を判定する。
package geo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"unicode/utf8"
)

type ring struct {
	points []intPoint
	box    rectangle
}

type polygon struct {
	rings []ring
	box   rectangle
}

type area struct {
	code     string
	polygons []polygon
	box      rectangle
}

// Boundaries は ReadN03 で読み込んだ統合後の区域を保持する。
type Boundaries struct {
	areas  []area
	meshes []string
}

// ReadN03 は各 ZIP の GeoJSON を読み、rollup の区を市にまとめる。
// Reader は閉じず、失敗時には部分結果を返さない。
func ReadN03(sources []ZIPSource, rollup map[string]string) (*Boundaries, error) {
	if err := validateSources(sources); err != nil {
		return nil, err
	}
	if err := validateRollup(rollup); err != nil {
		return nil, err
	}
	byCode := make(map[string]*area)
	for _, src := range sources {
		data, where, err := n03Bytes(src)
		if err != nil {
			return nil, err
		}
		if err := readFeatures(data, where, rollup, byCode); err != nil {
			return nil, err
		}
	}
	b := &Boundaries{areas: make([]area, 0, len(byCode))}
	for _, a := range byCode {
		b.areas = append(b.areas, *a)
	}
	slices.SortFunc(b.areas, func(a, b area) int {
		if a.code < b.code {
			return -1
		}
		if a.code > b.code {
			return 1
		}
		return 0
	})
	b.meshes = primaryMeshes(b.areas)
	return b, nil
}

func areaCode(s string) bool {
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

func validateRollup(rollup map[string]string) error {
	for from, to := range rollup {
		_, chained := rollup[to]
		if !areaCode(from) || !areaCode(to) || from == to || chained {
			return fmt.Errorf("%w: invalid, self-referencing or chained rollup %q -> %q", ErrInvalidArgument, from, to)
		}
	}
	return nil
}

func decodeJSON(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(dst); err != nil {
		return errors.Join(ErrInvalidFormat, err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return invalid("multiple JSON values")
		}
		return errors.Join(ErrInvalidFormat, err)
	}
	return nil
}

func readFeatures(data []byte, where ReadError, rollup map[string]string, byCode map[string]*area) error {
	if !utf8.Valid(data) {
		return where.fail("JSON", invalid("invalid UTF-8"))
	}
	var collection struct {
		Type string `json:"type"`
		CRS  struct {
			Type       string `json:"type"`
			Properties struct {
				Name string `json:"name"`
			} `json:"properties"`
		} `json:"crs"`
		Features json.RawMessage `json:"features"`
	}
	if err := decodeJSON(data, &collection); err != nil {
		return where.fail("JSON", err)
	}
	if collection.Type != "FeatureCollection" {
		return where.fail("Type", invalid("expected FeatureCollection"))
	}
	crs := collection.CRS.Properties.Name
	if collection.CRS.Type != "name" || (crs != "EPSG:6668" && crs != "urn:ogc:def:crs:EPSG::6668") {
		return where.fail("CRS", invalid("expected EPSG:6668"))
	}
	if len(collection.Features) == 0 || collection.Features[0] != '[' {
		return where.fail("Features", invalid("expected features array"))
	}
	var features []json.RawMessage
	if err := decodeJSON(collection.Features, &features); err != nil {
		return where.fail("Features", err)
	}
	for i, raw := range features {
		base := fmt.Sprintf("Features[%d]", i)
		var f struct {
			Type       string                     `json:"type"`
			Properties map[string]json.RawMessage `json:"properties"`
			Geometry   struct {
				Type        string `json:"type"`
				Coordinates any    `json:"coordinates"`
			} `json:"geometry"`
		}
		if err := decodeJSON(raw, &f); err != nil {
			return where.fail(base, err)
		}
		if f.Type != "Feature" {
			return where.fail(base+".Type", invalid("expected Feature"))
		}
		codeJSON, ok := f.Properties["N03_007"]
		if !ok {
			return where.fail(base+".Properties.N03_007", invalid("missing area code"))
		}
		var code string
		if err := decodeJSON(codeJSON, &code); err != nil {
			return where.fail(base+".Properties.N03_007", err)
		}
		where.Key = code
		if code != "" && !areaCode(code) {
			return where.fail(base+".Properties.N03_007", invalid("expected five-digit area code"))
		}
		polygons, err := readGeometry(f.Geometry.Type, f.Geometry.Coordinates)
		if err != nil {
			return where.fail(base+".Geometry", err)
		}
		if code == "" {
			where.Key = ""
			continue
		}
		if to, ok := rollup[code]; ok {
			code = to
		}
		a := byCode[code]
		if a == nil {
			a = &area{code: code, box: polygons[0].box}
			byCode[code] = a
		}
		for _, p := range polygons {
			a.polygons = append(a.polygons, p)
			a.box = a.box.include(p.box)
		}
		where.Key = ""
	}
	return nil
}

func readGeometry(kind string, coordinates any) ([]polygon, error) {
	values, ok := coordinates.([]any)
	if !ok || len(values) == 0 {
		return nil, invalid("empty or invalid coordinates")
	}
	switch kind {
	case "Polygon":
		values = []any{values}
	case "MultiPolygon":
	default:
		return nil, invalid("expected Polygon or MultiPolygon")
	}
	result := make([]polygon, 0, len(values))
	for _, value := range values {
		rawRings, ok := value.([]any)
		if !ok || len(rawRings) == 0 {
			return nil, invalid("empty or invalid polygon")
		}
		p := polygon{}
		for _, raw := range rawRings {
			r, err := readRing(raw)
			if err != nil {
				return nil, err
			}
			p.rings = append(p.rings, r)
		}
		p.box = p.rings[0].box
		result = append(result, p)
	}
	return result, nil
}

func readRing(raw any) (ring, error) {
	values, ok := raw.([]any)
	if !ok || len(values) < 4 {
		return ring{}, invalid("ring needs at least four positions")
	}
	r := ring{points: make([]intPoint, 0, len(values)-1)}
	var first, previous intPoint
	var sum, term big.Int
	distinct := make(map[intPoint]struct{}, 3)
	for i, value := range values {
		pair, ok := value.([]any)
		if !ok || len(pair) != 2 {
			return ring{}, invalid("position must have two coordinates")
		}
		var p intPoint
		for j, target := range []*int64{&p.x, &p.y} {
			n, ok := pair[j].(json.Number)
			if !ok {
				return ring{}, invalid("coordinate must be a JSON number")
			}
			v, err := decimalCoordinate(n)
			if err != nil {
				return ring{}, err
			}
			limit := int64(180)
			if j == 1 {
				limit = 90
			}
			if v < -limit*decimalScale || v > limit*decimalScale {
				return ring{}, invalid("coordinate out of range")
			}
			*target = 3 * v
		}
		if i == 0 {
			first = p
			r.box = rectangle{p.x, p.y, p.x, p.y}
		} else {
			product(previous.x, p.y).sub(product(previous.y, p.x)).bigInt(&term)
			sum.Add(&sum, &term)
		}
		r.box = r.box.include(rectangle{p.x, p.y, p.x, p.y})
		if len(distinct) < 3 {
			distinct[p] = struct{}{}
		}
		if i < len(values)-1 {
			r.points = append(r.points, p)
		}
		previous = p
	}
	if previous != first || len(distinct) < 3 || sum.Sign() == 0 {
		return ring{}, invalid("ring must be closed with three distinct points and nonzero area")
	}
	return r, nil
}
