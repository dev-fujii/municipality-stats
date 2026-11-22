package jma

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

func sjis(t testing.TB, s string) string {
	t.Helper()
	b, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReadStations(t *testing.T) {
	m, i := fixtureEntries(t)
	ds, err := Read(memoryZIP(t, m), memoryZIP(t, i), testElements)
	if err != nil {
		t.Fatal(err)
	}
	height := int64(100)
	want := Station{
		ID: "49236", Name: "切石", NameKana: "ｷﾘｲｼ", NameRoman: "KIRIISHI",
		Latitude: Coordinate{"35", "28.0", 21280}, Longitude: Coordinate{"138", "26.5", 83065},
		AltitudeMeters: 226, AnemometerHeightTenths: &height,
		Precipitation: ObservationMeasured, Wind: ObservationMeasured, Temperature: ObservationMeasured, Sunshine: ObservationEstimated, SnowDepth: ObservationNone,
	}
	if !reflect.DeepEqual(ds.Stations[0].Station, want) {
		t.Fatalf("station=%+v, want %+v", ds.Stations[0].Station, want)
	}
	if ds.Stations[1].Station.Name != "河口湖" || *ds.Stations[1].Station.AnemometerHeightTenths != 151 || ds.Stations[2].Station.AnemometerHeightTenths != nil {
		t.Fatalf("stations=%+v", ds.Stations)
	}
	for _, tc := range []struct {
		station int
		mark    ObservationMark
		status  Status
	}{{0, ObservationEstimated, StatusOK}, {1, ObservationMeasured, StatusOK}, {2, ObservationMeasured, StatusUnavailable}, {3, ObservationNone, StatusUnavailable}} {
		s := ds.Stations[tc.station]
		if s.Station.Sunshine != tc.mark || s.Normals[3].Monthly[0].Status != tc.status {
			t.Fatalf("sunshine=%+v", s)
		}
	}
	row, station := firstRows(t)
	for _, tc := range []struct {
		name    string
		changes map[int]string
		check   func(*testing.T, Station)
	}{
		{"negative altitude", map[int]string{8: "-003"}, func(t *testing.T, s Station) {
			if s.AltitudeMeters != -3 {
				t.Fatal(s)
			}
		}},
		{"height", map[int]string{9: "010.1"}, func(t *testing.T, s Station) {
			if s.AnemometerHeightTenths == nil || *s.AnemometerHeightTenths != 101 {
				t.Fatal(s)
			}
		}},
		{"height zero", map[int]string{9: "000.0"}, func(t *testing.T, s Station) {
			if s.AnemometerHeightTenths == nil || *s.AnemometerHeightTenths != 0 {
				t.Fatal(s)
			}
		}},
		{"height blank", map[int]string{9: "     "}, func(t *testing.T, s Station) {
			if s.AnemometerHeightTenths != nil {
				t.Fatal(s)
			}
		}},
		{"coordinates leading zero", map[int]string{4: "02", 5: "03.4", 6: "003", 7: "05.6"}, func(t *testing.T, s Station) {
			if s.Latitude != (Coordinate{"02", "03.4", 1234}) || s.Longitude != (Coordinate{"003", "05.6", 1856}) {
				t.Fatal(s)
			}
		}},
		{"coordinate endpoints", map[int]string{4: "90", 5: "00.0", 6: "180", 7: "00.0"}, func(t *testing.T, s Station) {
			if s.Latitude.TotalMinutesTenths != 54000 || s.Longitude.TotalMinutesTenths != 108000 {
				t.Fatal(s)
			}
		}},
		{"coordinate zero", map[int]string{4: "00", 5: "00.0", 6: "000", 7: "00.0"}, func(t *testing.T, s Station) {
			if s.Latitude.TotalMinutesTenths != 0 || s.Longitude.TotalMinutesTenths != 0 {
				t.Fatal(s)
			}
		}},
		{"minutes upper bound", map[int]string{5: "59.9", 7: "59.9"}, func(t *testing.T, s Station) {
			if s.Latitude.TotalMinutesTenths != 21599 || s.Longitude.TotalMinutesTenths != 83399 {
				t.Fatal(s)
			}
		}},
		{"names internal spaces", map[int]string{1: sjis(t, "　切　石"+strings.Repeat("　", 6)), 2: " A B" + strings.Repeat(" ", 11), 3: " A B" + strings.Repeat(" ", 26)}, func(t *testing.T, s Station) {
			if s.Name != "　切　石" || s.NameKana != " A B" || s.NameRoman != " A B" {
				t.Fatal(s)
			}
		}},
		{"Shift_JIS trail backslash", map[int]string{1: sjis(t, "表"+strings.Repeat("　", 9))}, func(t *testing.T, s Station) {
			if s.Name != "表" {
				t.Fatal(s)
			}
		}},
		{"mark independent of RMK", map[int]string{12: "0", 13: "0"}, func(t *testing.T, s Station) {
			if s.Temperature != ObservationNone || s.Sunshine != ObservationNone {
				t.Fatal(s)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bytes.Clone(station)
			for col, value := range tc.changes {
				b = replaceCol(b, col, value)
			}
			ds, err := readRows(t, row, b)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, ds.Stations[0].Station)
			if ds.Stations[0].Normals[0].Monthly[0].Status != StatusOK {
				t.Fatal("station mark changed RMK status")
			}
		})
	}
	for col := 10; col <= 14; col++ {
		for _, mark := range []string{"0", "1", "2"} {
			if mark == "2" && col != 13 {
				continue
			}
			t.Run(fmt.Sprintf("mark %d=%s", col, mark), func(t *testing.T) {
				_, err := readRows(t, row, replaceCol(station, col, mark))
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestReadStationErrors(t *testing.T) {
	row, station := firstRows(t)
	for _, tc := range []struct {
		name         string
		col          int
		value, field string
	}{
		{"ID non-digit", 0, "4923x", "ID"},
		{"ID fullwidth", 0, " 9236", "ID"},
		{"empty kanji", 1, sjis(t, strings.Repeat("　", 10)), "Name"},
		{"empty kana", 2, strings.Repeat(" ", 15), "NameKana"},
		{"empty roman", 3, strings.Repeat(" ", 30), "NameRoman"},
		{"quote", 3, "\"" + strings.Repeat(" ", 29), "Line"},
		{"escape", 3, "\\" + strings.Repeat(" ", 29), "Line"},
		{"latitude sign", 4, "+5", "Latitude"},
		{"latitude padding", 4, " 5", "Latitude"},
		{"latitude degrees", 4, "91", "Latitude"},
		{"latitude minutes shape", 5, "2800", "Latitude"},
		{"latitude minutes sign", 5, "-0.1", "Latitude"},
		{"latitude minutes", 5, "60.0", "Latitude"},
		{"longitude sign", 6, "+38", "Longitude"},
		{"longitude degrees", 6, "181", "Longitude"},
		{"longitude minutes shape", 7, "28e0", "Longitude"},
		{"longitude minutes", 7, "60.0", "Longitude"},
		{"altitude plus", 8, "+003", "AltitudeMeters"},
		{"altitude padding", 8, "   3", "AltitudeMeters"},
		{"altitude decimal", 8, "00.3", "AltitudeMeters"},
		{"height padding", 9, " 10.1", "AnemometerHeightTenths"},
		{"height sign", 9, "-10.1", "AnemometerHeightTenths"},
		{"height exponent", 9, "01e01", "AnemometerHeightTenths"},
		{"height point", 9, "01001", "AnemometerHeightTenths"},
		{"precipitation estimated", 10, "2", "Precipitation"},
		{"wind estimated", 11, "2", "Wind"},
		{"temperature estimated", 12, "2", "Temperature"},
		{"snow estimated", 14, "2", "SnowDepth"},
		{"sunshine three", 13, "3", "Sunshine"},
		{"sunshine blank", 13, " ", "Sunshine"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, err := readRows(t, row, replaceCol(station, tc.col, tc.value))
			id := "49236"
			if tc.field == "ID" || tc.name == "quote" {
				id = ""
			}
			checkReadError(t, ds, err, "index", "amedas_station_index.csv", 3, id, "", tc.field)
		})
	}
	for _, tc := range []struct {
		name      string
		change    func([]byte) []byte
		field, id string
	}{
		{"short", func(b []byte) []byte { return b[:110] }, "Line", ""},
		{"long", func(b []byte) []byte { return append(b, ' ') }, "Line", ""},
		{"wrong column count", func(b []byte) []byte { b[5] = 'x'; return b }, "Line", ""},
		{"wrong column widths", func(b []byte) []byte { b[5], b[6] = b[6], b[5]; return b }, "Line", ""},
		{"invalid byte", func(b []byte) []byte { b[6] = 0xff; return b }, "Line", "49236"},
		{"truncated Shift_JIS", func(b []byte) []byte { b[24], b[25] = ' ', 0x81; return b }, "Line", "49236"},
		{"latitude pole nonzero", func(b []byte) []byte { return replaceCol(replaceCol(b, 4, "90"), 5, "00.1") }, "Latitude", "49236"},
		{"longitude end nonzero", func(b []byte) []byte { return replaceCol(replaceCol(b, 6, "180"), 7, "00.1") }, "Longitude", "49236"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, err := readRows(t, row, tc.change(bytes.Clone(station)))
			checkReadError(t, ds, err, "index", "amedas_station_index.csv", 3, tc.id, "", tc.field)
		})
	}
	for _, tc := range []struct {
		name      string
		data      []byte
		line      int
		id, field string
	}{
		{"empty", nil, 0, "", "Header"},
		{"one header", []byte(stationHeaders[0]), 0, "", "Header"},
		{"no data", indexCSV(), 0, "", "Line"},
		{"header modified", bytes.Replace(indexCSV(station), []byte("Station Number"), []byte("Station number"), 1), 1, "", "Header"},
		{"header two missing", bytes.Join([][]byte{[]byte(stationHeaders[0]), station}, []byte{'\n'}), 2, "", "Header"},
		{"header two columns padded", []byte(stationHeaders[0] + "\n" + stationHeaders[1] + ",,,,,\n"), 2, "", "Header"},
		{"header BOM", append([]byte{0xef, 0xbb, 0xbf}, indexCSV(station)...), 1, "", "Header"},
		{"duplicate", indexCSV(station, station), 4, "49236", "ID"},
		{"empty middle row", indexCSV(nil, station), 3, "", "Line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, err := Read(memoryZIP(t, []testEntry{{monthlyPath("49236"), row}}), memoryZIP(t, []testEntry{{"amedas_station_index.csv", tc.data}}), []Element{ElementTemperature})
			checkReadError(t, ds, err, "index", "amedas_station_index.csv", tc.line, tc.id, "", tc.field)
		})
	}
}
