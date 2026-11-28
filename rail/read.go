package rail

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

// N02 の主な事業者種別コード（N02_002）。
const (
	InstitutionShinkansen = "1"
	InstitutionJR         = "2"
)

// ZIPSource は読み込み対象の ZIP を表す。Reader の開閉は呼び出す側が行う。
type ZIPSource struct {
	Name   string
	Reader io.ReaderAt
	Size   int64
}

// Point は緯度・経度（度）を表す。
type Point struct {
	Latitude  float64
	Longitude float64
}

// Record は国土数値情報 N02 の駅の1地物を表す。
type Record struct {
	InstitutionType string
	LineName        string
	Operator        string
	StationName     string
	StationCode     string
	GroupCode       string
	Coordinates     []Point
}

// ReadZip は ZIP 内の UTF-8/ 以下にある *_Station.geojson を1つ見つけて読み、駅のレコードを返す。
// Reader は閉じず、失敗時には部分結果を返さない。
func ReadZip(src ZIPSource) ([]Record, error) {
	if nilInterface(src.Reader) || src.Size <= 0 {
		return nil, fmt.Errorf("%w: source %q requires a non-nil Reader and positive Size", ErrInvalidArgument, src.Name)
	}
	where := ReadError{Source: src.Name}
	z, err := zip.NewReader(src.Reader, src.Size)
	if err != nil {
		return nil, where.fail("File", err)
	}
	var selected *zip.File
	for _, f := range z.File {
		if f.FileInfo().IsDir() || !isStationGeoJSONFile(f.Name) {
			continue
		}
		where.File = f.Name
		if selected != nil {
			return nil, where.fail("File", invalid("multiple Station GeoJSON files"))
		}
		selected = f
	}
	if selected == nil {
		return nil, where.fail("File", invalid("Station GeoJSON file missing"))
	}
	r, err := selected.Open()
	if err != nil {
		return nil, where.fail("File", err)
	}
	data, err := readAllAndClose(r)
	if err != nil {
		return nil, where.fail("File", err)
	}
	return parseGeoJSON(data, where)
}

// ReadGeoJSON は UTF-8 の N02 駅 GeoJSON を読み、駅のレコードを返す。
// Reader は閉じず、失敗時には部分結果を返さない。
func ReadGeoJSON(r io.Reader) ([]Record, error) {
	if nilInterface(r) {
		return nil, fmt.Errorf("%w: reader must not be nil", ErrInvalidArgument)
	}
	var where ReadError
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, where.fail("JSON", err)
	}
	return parseGeoJSON(data, where)
}

func isStationGeoJSONFile(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	base := path.Base(normalized)
	if !strings.HasSuffix(base, "_Station.geojson") || base == "_Station.geojson" {
		return false
	}
	dir := path.Dir(normalized)
	return slices.Contains(strings.Split(dir, "/"), "UTF-8")
}

func readAllAndClose(r io.ReadCloser) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err = errors.Join(err, r.Close()); err != nil {
		return nil, err
	}
	return data, nil
}

func nilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

type featureCollection struct {
	Type     string          `json:"type"`
	Features json.RawMessage `json:"features"`
}

type rawFeature struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
	Geometry   json.RawMessage `json:"geometry"`
}

type rawGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

func parseGeoJSON(data []byte, where ReadError) ([]Record, error) {
	if !utf8.Valid(data) {
		return nil, where.fail("JSON", invalid("invalid UTF-8"))
	}
	var fc featureCollection
	if err := decodeJSON(data, &fc); err != nil {
		return nil, where.fail("JSON", err)
	}
	if fc.Type != "FeatureCollection" {
		return nil, where.fail("Type", invalid("unexpected GeoJSON type %q", fc.Type))
	}
	rawFeatures, err := decodeArray(fc.Features)
	if err != nil {
		return nil, where.fail("Features", err)
	}

	records := make([]Record, 0, len(rawFeatures))
	for i, raw := range rawFeatures {
		base := fmt.Sprintf("Features[%d]", i)
		rec, err := parseFeature(raw, where, base)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, nil
}

func parseFeature(raw json.RawMessage, where ReadError, base string) (Record, error) {
	var f rawFeature
	if err := decodeObject(raw, &f); err != nil {
		return Record{}, where.fail(base, err)
	}
	if f.Type != "Feature" {
		return Record{}, where.fail(base+".Type", invalid("unexpected feature type %q", f.Type))
	}

	var props map[string]any
	if err := decodeObject(f.Properties, &props); err != nil {
		return Record{}, where.fail(base+".Properties", err)
	}

	groupCode, err := requireString(props, "N02_005g")
	if err != nil {
		return Record{}, where.fail(base+".Properties.N02_005g", err)
	}
	where.Key = groupCode

	instType, err := requireString(props, "N02_002")
	if err != nil {
		return Record{}, where.fail(base+".Properties.N02_002", err)
	}
	lineName, err := requireString(props, "N02_003")
	if err != nil {
		return Record{}, where.fail(base+".Properties.N02_003", err)
	}
	operator, err := requireString(props, "N02_004")
	if err != nil {
		return Record{}, where.fail(base+".Properties.N02_004", err)
	}
	stationName, err := requireString(props, "N02_005")
	if err != nil {
		return Record{}, where.fail(base+".Properties.N02_005", err)
	}
	stationCode, err := requireString(props, "N02_005c")
	if err != nil {
		return Record{}, where.fail(base+".Properties.N02_005c", err)
	}

	coords, err := parseLineStringGeometry(f.Geometry, where, base+".Geometry")
	if err != nil {
		return Record{}, err
	}

	return Record{
		InstitutionType: instType,
		LineName:        lineName,
		Operator:        operator,
		StationName:     stationName,
		StationCode:     stationCode,
		GroupCode:       groupCode,
		Coordinates:     coords,
	}, nil
}

func parseLineStringGeometry(raw json.RawMessage, where ReadError, base string) ([]Point, error) {
	var g rawGeometry
	if err := decodeObject(raw, &g); err != nil {
		return nil, where.fail(base, err)
	}
	if g.Type != "LineString" {
		return nil, where.fail(base+".Type", invalid("unexpected geometry type %q", g.Type))
	}
	rawCoords, err := decodeArray(g.Coordinates)
	if err != nil {
		return nil, where.fail(base+".Coordinates", err)
	}
	if len(rawCoords) < 2 {
		return nil, where.fail(base+".Coordinates", invalid("LineString must have at least 2 coordinates, got %d", len(rawCoords)))
	}
	coords := make([]Point, 0, len(rawCoords))
	for _, rc := range rawCoords {
		pair, err := decodeArray(rc)
		if err != nil {
			return nil, where.fail(base+".Coordinates", err)
		}
		if len(pair) != 2 {
			return nil, where.fail(base+".Coordinates", invalid("coordinate must have 2 elements, got %d", len(pair)))
		}
		lon, err := parseFloatNumber(pair[0])
		if err != nil {
			return nil, where.fail(base+".Coordinates", err)
		}
		lat, err := parseFloatNumber(pair[1])
		if err != nil {
			return nil, where.fail(base+".Coordinates", err)
		}
		pt := Point{Latitude: lat, Longitude: lon}
		if !validPoint(pt) {
			return nil, where.fail(base+".Coordinates", invalid("coordinate out of range"))
		}
		coords = append(coords, pt)
	}
	return coords, nil
}

func decodeJSON(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(dst); err != nil {
		return errors.Join(ErrInvalidFormat, err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return invalid("trailing JSON data")
	}
	return nil
}

func decodeObject(raw json.RawMessage, dst any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return invalid("expected JSON object")
	}
	return decodeJSON(trimmed, dst)
}

func decodeArray(raw json.RawMessage) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, invalid("expected JSON array")
	}
	var items []json.RawMessage
	if err := decodeJSON(trimmed, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func parseFloatNumber(raw json.RawMessage) (float64, error) {
	var v any
	if err := decodeJSON(raw, &v); err != nil {
		return 0, err
	}
	num, ok := v.(json.Number)
	if !ok {
		return 0, invalid("coordinate must be a number")
	}
	f, err := num.Float64()
	if err != nil {
		return 0, errors.Join(invalid("invalid coordinate number"), err)
	}
	return f, nil
}

func requireString(props map[string]any, key string) (string, error) {
	v, ok := props[key]
	if !ok || v == nil {
		return "", invalid("missing %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", invalid("%s must be a string", key)
	}
	if s == "" {
		return "", invalid("%s must not be empty", key)
	}
	return s, nil
}

func validPoint(p Point) bool {
	if math.IsNaN(p.Latitude) || math.IsInf(p.Latitude, 0) || math.IsNaN(p.Longitude) || math.IsInf(p.Longitude, 0) {
		return false
	}
	return p.Latitude >= -90 && p.Latitude <= 90 && p.Longitude >= -180 && p.Longitude <= 180
}
