package jma

import (
	"archive/zip"
	"regexp"
	"strings"
)

// Coordinate は、度・分の原文と0.1分単位の整数を持つ。
type Coordinate struct {
	DegreesRaw         string
	MinutesRaw         string
	TotalMinutesTenths int64
}

// ObservationMark は、観測なし、観測、推計の印を表す。
type ObservationMark int

const (
	ObservationNone      ObservationMark = 0
	ObservationMeasured  ObservationMark = 1
	ObservationEstimated ObservationMark = 2
)

// Station は、観測地点の名前、位置、高度と要素ごとの観測の印を持つ。
// AnemometerHeightTenths は0.1m単位で、記載が空白なら nil。
type Station struct {
	ID                     string
	Name                   string
	NameKana               string
	NameRoman              string
	Latitude               Coordinate
	Longitude              Coordinate
	AltitudeMeters         int64
	AnemometerHeightTenths *int64
	Precipitation          ObservationMark
	Wind                   ObservationMark
	Temperature            ObservationMark
	Sunshine               ObservationMark
	SnowDepth              ObservationMark
}

var stationHeaders = [2]string{
	"Station Number,Station Name,Station Name,Station Name,Latitude,Latitude,Longitude,Longitude,Altitude,Height of Anemometer,Precipitation,Wind Speed,Temperature,Sunshine Duration,Depth of Snow Cover",
	",(Kanji),(Kana),,(degree),(minutes),(degree),(minutes),(m),(m)",
}

var (
	stationWidths   = []int{5, 20, 15, 30, 2, 4, 3, 4, 4, 5, 1, 1, 1, 1, 1}
	minutesPattern  = regexp.MustCompile(`^[0-9]{2}\.[0-9]$`)
	heightPattern   = regexp.MustCompile(`^[0-9]{3}\.[0-9]$`)
	altitudePattern = regexp.MustCompile(`^([0-9]{4}|-[0-9]{3})$`)
)

func readStations(f *zip.File) (map[string]Station, error) {
	where := ReadError{Archive: "index", File: f.Name}
	data, err := fileBytes(f, where)
	if err != nil {
		return nil, err
	}
	stations := make(map[string]Station)
	lines := 0
	err = eachLine(data, func(line int, row []byte) error {
		lines = line
		at := where
		at.Line = line
		if line <= 2 {
			if string(row) != stationHeaders[line-1] {
				return at.fail("Header", invalid("unexpected header"))
			}
			return nil
		}
		s, err := parseStation(row, at)
		if err != nil {
			return err
		}
		if _, ok := stations[s.ID]; ok {
			at.StationID = s.ID
			return at.fail("ID", invalid("duplicate station"))
		}
		stations[s.ID] = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	if lines < 2 {
		return nil, where.fail("Header", invalid("missing header"))
	}
	if len(stations) == 0 {
		return nil, where.fail("Line", invalid("no station data"))
	}
	return stations, nil
}

func parseStation(row []byte, where ReadError) (Station, error) {
	var s Station
	cols, err := columns(row, 111, stationWidths)
	if err != nil {
		return s, where.fail("Line", err)
	}
	if !digitsPattern.Match(cols[0]) {
		return s, where.fail("ID", invalid("station number must be five ASCII digits"))
	}
	s.ID = string(cols[0])
	where.StationID = s.ID
	if _, err := decodeSJIS(row); err != nil {
		return s, where.fail("Line", err)
	}
	for i, field := range []struct {
		name string
		pad  string
		dest *string
	}{{"Name", "\u3000", &s.Name}, {"NameKana", " ", &s.NameKana}, {"NameRoman", " ", &s.NameRoman}} {
		name, err := decodeSJIS(cols[i+1])
		if err != nil {
			return s, where.fail(field.name, err)
		}
		*field.dest = strings.TrimRight(name, field.pad)
		if *field.dest == "" {
			return s, where.fail(field.name, invalid("empty station name"))
		}
	}
	s.Latitude, err = parseCoordinate(string(cols[4]), string(cols[5]), 90)
	if err != nil {
		return s, where.fail("Latitude", err)
	}
	s.Longitude, err = parseCoordinate(string(cols[6]), string(cols[7]), 180)
	if err != nil {
		return s, where.fail("Longitude", err)
	}
	if !altitudePattern.Match(cols[8]) {
		return s, where.fail("AltitudeMeters", invalid("invalid altitude"))
	}
	s.AltitudeMeters, err = parseInteger(string(cols[8]), true, false)
	if err != nil {
		return s, where.fail("AltitudeMeters", err)
	}
	height := string(cols[9])
	if height != "     " {
		if !heightPattern.MatchString(height) {
			return s, where.fail("AnemometerHeightTenths", invalid("invalid anemometer height"))
		}
		x, err := parseInteger(strings.ReplaceAll(height, ".", ""), false, false)
		if err != nil {
			return s, where.fail("AnemometerHeightTenths", err)
		}
		s.AnemometerHeightTenths = &x
	}
	for i, field := range []struct {
		name string
		dest *ObservationMark
	}{{"Precipitation", &s.Precipitation}, {"Wind", &s.Wind}, {"Temperature", &s.Temperature}, {"Sunshine", &s.Sunshine}, {"SnowDepth", &s.SnowDepth}} {
		mark := cols[10+i][0]
		if mark != '0' && mark != '1' && (i != 3 || mark != '2') {
			return s, where.fail(field.name, invalid("invalid observation mark %q", mark))
		}
		*field.dest = ObservationMark(mark - '0')
	}
	return s, nil
}

func parseCoordinate(degrees, minutes string, limit int64) (Coordinate, error) {
	var c Coordinate
	if !digitsPattern.MatchString(degrees) || !minutesPattern.MatchString(minutes) {
		return c, invalid("invalid coordinate notation")
	}
	d, err := parseInteger(degrees, false, false)
	if err != nil {
		return c, err
	}
	m, err := parseInteger(strings.ReplaceAll(minutes, ".", ""), false, false)
	if err != nil {
		return c, err
	}
	if d > limit || m > 599 || (d == limit && m != 0) {
		return c, invalid("coordinate out of range")
	}
	return Coordinate{DegreesRaw: degrees, MinutesRaw: minutes, TotalMinutesTenths: d*600 + m}, nil
}
