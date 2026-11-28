package landprice

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
	"strconv"
	"strings"
	"unicode/utf8"
)

// L02 の主な用途区分コード（L02_001）。
const (
	UseResidential        = "000"
	UseProspectiveHousing = "003"
	UseCommercial         = "005"
	UseSemiIndustrial     = "007"
	UseIndustrial         = "009"
	UseControlAreaHousing = "010"
	UseForest             = "020"
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

// Record は都道府県地価調査（L02）の基準地1件を表す。
type Record struct {
	UseCode   string
	Serial    string
	Year      int
	Price     int64
	AreaCode  string
	PlaceName string
	Point     Point
}

// ReadZip は ZIP 内の *.geojson を1つ見つけて読み、基準地の並びを返す。
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
		if f.FileInfo().IsDir() || !isGeoJSONFile(f.Name) {
			continue
		}
		where.File = f.Name
		if selected != nil {
			return nil, where.fail("File", invalid("multiple GeoJSON files"))
		}
		selected = f
	}
	if selected == nil {
		return nil, where.fail("File", invalid("GeoJSON file missing"))
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

// ReadGeoJSON は UTF-8 の L02 GeoJSON を読み、基準地の並びを返す。
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

func isGeoJSONFile(name string) bool {
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	return strings.HasSuffix(base, ".geojson") && base != ".geojson"
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
	var expectedYear int
	for i, raw := range rawFeatures {
		base := fmt.Sprintf("Features[%d]", i)
		rec, err := parseFeature(raw, where, base)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			expectedYear = rec.Year
		} else if rec.Year != expectedYear {
			w := where
			w.Key = rec.AreaCode
			return nil, w.fail(base+".Properties.L02_005", invalid("mixed survey years %d and %d", expectedYear, rec.Year))
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

	areaCode, err := requireString(props, "L02_020", false)
	if err != nil {
		return Record{}, where.fail(base+".Properties.L02_020", err)
	}
	if !fiveDigits(areaCode) {
		return Record{}, where.fail(base+".Properties.L02_020", invalid("L02_020 must be 5 ASCII digits, got %q", areaCode))
	}
	where.Key = areaCode

	useCode, err := requireString(props, "L02_001", false)
	if err != nil {
		return Record{}, where.fail(base+".Properties.L02_001", err)
	}
	serial, err := requireString(props, "L02_002", false)
	if err != nil {
		return Record{}, where.fail(base+".Properties.L02_002", err)
	}
	year64, err := requireInt64(props, "L02_005")
	if err != nil {
		return Record{}, where.fail(base+".Properties.L02_005", err)
	}
	if year64 <= 0 || year64 > math.MaxInt {
		return Record{}, where.fail(base+".Properties.L02_005", invalid("L02_005 must be a positive year, got %d", year64))
	}
	price, err := requireInt64(props, "L02_006")
	if err != nil {
		return Record{}, where.fail(base+".Properties.L02_006", err)
	}
	if price < 0 {
		return Record{}, where.fail(base+".Properties.L02_006", invalid("L02_006 must not be negative, got %d", price))
	}
	placeName, err := requireString(props, "L02_021", true)
	if err != nil {
		return Record{}, where.fail(base+".Properties.L02_021", err)
	}

	pt, err := parsePointGeometry(f.Geometry, where, base+".Geometry")
	if err != nil {
		return Record{}, err
	}

	return Record{
		UseCode:   useCode,
		Serial:    serial,
		Year:      int(year64),
		Price:     price,
		AreaCode:  areaCode,
		PlaceName: placeName,
		Point:     pt,
	}, nil
}

func parsePointGeometry(raw json.RawMessage, where ReadError, base string) (Point, error) {
	var g rawGeometry
	if err := decodeObject(raw, &g); err != nil {
		return Point{}, where.fail(base, err)
	}
	if g.Type != "Point" {
		return Point{}, where.fail(base+".Type", invalid("unexpected geometry type %q", g.Type))
	}
	elems, err := decodeArray(g.Coordinates)
	if err != nil {
		return Point{}, where.fail(base+".Coordinates", err)
	}
	if len(elems) != 2 {
		return Point{}, where.fail(base+".Coordinates", invalid("Point coordinates must have 2 elements, got %d", len(elems)))
	}
	lon, err := parseFloatNumber(elems[0])
	if err != nil {
		return Point{}, where.fail(base+".Coordinates", err)
	}
	lat, err := parseFloatNumber(elems[1])
	if err != nil {
		return Point{}, where.fail(base+".Coordinates", err)
	}
	pt := Point{Latitude: lat, Longitude: lon}
	if !validPoint(pt) {
		return Point{}, where.fail(base+".Coordinates", invalid("coordinate out of range"))
	}
	return pt, nil
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

func requireString(props map[string]any, key string, allowEmpty bool) (string, error) {
	v, ok := props[key]
	if !ok || v == nil {
		return "", invalid("missing %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", invalid("%s must be a string", key)
	}
	if !allowEmpty && s == "" {
		return "", invalid("%s must not be empty", key)
	}
	return s, nil
}

func requireInt64(props map[string]any, key string) (int64, error) {
	v, ok := props[key]
	if !ok || v == nil {
		return 0, invalid("missing %s", key)
	}
	num, ok := v.(json.Number)
	if !ok {
		return 0, invalid("%s must be an integer", key)
	}
	n, err := strconv.ParseInt(num.String(), 10, 64)
	if err != nil {
		return 0, errors.Join(invalid("%s must be an integer", key), err)
	}
	return n, nil
}

func validPoint(p Point) bool {
	if math.IsNaN(p.Latitude) || math.IsInf(p.Latitude, 0) || math.IsNaN(p.Longitude) || math.IsInf(p.Longitude, 0) {
		return false
	}
	return p.Latitude >= -90 && p.Latitude <= 90 && p.Longitude >= -180 && p.Longitude <= 180
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
