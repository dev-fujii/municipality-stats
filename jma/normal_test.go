package jma

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestReadNormalValues(t *testing.T) {
	m, i := fixtureEntries(t)
	ds, err := Read(memoryZIP(t, m), memoryZIP(t, i), testElements)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		station, element  int
		values            [13]int64
		years, start, end int
	}{
		{0, 0, [13]int64{18, 36, 76, 130, 177, 212, 250, 261, 222, 161, 97, 42, 140}, 30, 1991, 2020},
		{0, 2, [13]int64{-34, -21, 17, 69, 122, 171, 211, 220, 183, 121, 50, -9, 91}, 30, 1991, 2020},
		{0, 3, [13]int64{1886, 1785, 1900, 2000, 2018, 1461, 1686, 2058, 1485, 1467, 1640, 1806, 21204}, 30, 1991, 2020},
		{1, 4, [13]int64{25, 24, 16, 4, 0, 0, 0, 0, 0, 0, 1, 6, 36}, 30, 1991, 2020},
		{3, 0, [13]int64{56, 67, 99, 139, 185, 215, 252, 266, 234, 186, 134, 81, 160}, 12, 2009, 2020},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.station, tc.element), func(t *testing.T) {
			n := ds.Stations[tc.station].Normals[tc.element]
			if n.Years != tc.years || n.StartYear != tc.start || n.EndYear != tc.end {
				t.Fatalf("period=%+v", n)
			}
			for k, v := range append(n.Monthly[:], n.Annual) {
				if v != (Value{Integer: tc.values[k], RMK: 8, Status: StatusOK}) {
					t.Errorf("period %d=%+v", k, v)
				}
			}
		})
	}
	for _, tc := range []struct{ station, element int }{{0, 4}, {0, 5}, {3, 3}, {3, 4}, {3, 5}} {
		n := ds.Stations[tc.station].Normals[tc.element]
		if n.Years != 0 || n.StartYear != 0 || n.EndYear != 0 {
			t.Fatalf("unavailable period=%+v", n)
		}
		for _, v := range append(n.Monthly[:], n.Annual) {
			if v != (Value{RMK: 0, Status: StatusUnavailable}) {
				t.Errorf("unavailable value=%+v", v)
			}
		}
	}
	n := ds.Stations[2].Normals[3]
	if n.Years != 16 || n.StartYear != 2005 || n.EndYear != 2020 || n.Monthly[0].Status != StatusUnavailable || n.Monthly[7] != (Value{2237, 8, StatusOK}) || n.Annual.Status != StatusUnavailable {
		t.Fatalf("sunshine=%+v", n)
	}
}

func TestReadNormalNumbers(t *testing.T) {
	row, station := firstRows(t)
	for _, tc := range []struct {
		name              string
		changes           map[int]string
		integer           int64
		years, start, end int
	}{
		{"leading zeros", map[int]string{3: "09", 4: "1991", 5: "2020", 6: "000008"}, 8, 9, 1991, 2020},
		{"left padding", map[int]string{3: " 9", 6: "    -9"}, -9, 9, 1991, 2020},
		{"negative zero", map[int]string{6: "    -0"}, 0, 30, 1991, 2020},
		{"missing years allowed", map[int]string{3: "12"}, 18, 12, 1991, 2020},
		{"annual copied", map[int]string{30: "999999"}, 18, 30, 1991, 2020},
		{"unavailable retains years", map[int]string{6: "000000", 7: "0"}, 0, 30, 1991, 2020},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bytes.Clone(row)
			for col, value := range tc.changes {
				b = replaceCol(b, col, value)
			}
			ds, err := readRows(t, b, station)
			if err != nil {
				t.Fatal(err)
			}
			n := ds.Stations[0].Normals[0]
			if n.Monthly[0].Integer != tc.integer || n.Years != tc.years || n.StartYear != tc.start || n.EndYear != tc.end {
				t.Fatalf("normal=%+v", n)
			}
			if tc.name == "annual copied" && n.Annual.Integer != 999999 {
				t.Fatalf("annual=%+v", n.Annual)
			}
		})
	}
}

func TestReadNormalErrors(t *testing.T) {
	row, station := firstRows(t)
	for _, tc := range []struct {
		name         string
		col          int
		value, field string
	}{
		{"type", 0, "22", "Line"},
		{"station mismatch", 1, "49251", "StationID"},
		{"station non-digit", 1, "4923x", "StationID"},
		{"column count", 0, "2,", "Line"},
		{"column width", 0, "2", "Line"},
		{"quote", 6, "\"0018\"", "Line"},
		{"escape", 6, "\\00018", "Line"},
		{"years blank", 3, "  ", "Years"},
		{"years sign", 3, "+1", "Years"},
		{"years negative", 3, "-1", "Years"},
		{"years trailing space", 3, "1 ", "Years"},
		{"years range", 3, "31", "Years"},
		{"years too many", 4, "2019", "Years"},
		{"years zero", 3, " 0", "Years"},
		{"start blank", 4, "    ", "StartYear"},
		{"start plus", 4, "+991", "StartYear"},
		{"start small", 4, "1990", "StartYear"},
		{"start large", 4, "2021", "StartYear"},
		{"start zero only", 4, "   0", "StartYear"},
		{"end zero only", 5, "   0", "EndYear"},
		{"end earlier", 5, "1990", "EndYear"},
		{"end large", 5, "2021", "EndYear"},
		{"end blank", 5, "    ", "EndYear"},
		{"end exponent", 5, "2e03", "EndYear"},
		{"value blank", 6, "      ", "Monthly[0].Integer"},
		{"value decimal", 6, "  12.3", "Monthly[0].Integer"},
		{"value exponent", 6, "   1e3", "Monthly[0].Integer"},
		{"value plus", 6, "   +12", "Monthly[0].Integer"},
		{"value trailing", 6, "   12 ", "Monthly[0].Integer"},
		{"value interior", 6, "  1 23", "Monthly[0].Integer"},
		{"value minus", 6, "     -", "Monthly[0].Integer"},
		{"RMK seven", 7, "7", "Monthly[0].RMK"},
		{"RMK blank", 7, " ", "Monthly[0].RMK"},
		{"RMK nine", 7, "9", "Monthly[0].RMK"},
		{"unavailable nonzero", 7, "0", "Monthly[0].Integer"},
		{"annual invalid", 30, "   1.0", "Annual.Integer"},
		{"annual RMK", 31, "7", "Annual.RMK"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := replaceCol(row, tc.col, tc.value)
			field := tc.field
			element := ElementTemperature
			if len(b) != 143 {
				field = "Line"
				element = ""
			}
			ds, err := readRows(t, b, station)
			checkReadError(t, ds, err, "monthly", monthlyPath("49236"), 1, "49236", element, field)
		})
	}
	for _, tc := range []struct {
		name    string
		change  func([]byte) []byte
		line    int
		element Element
		field   string
	}{
		{"short", func(b []byte) []byte { return b[:142] }, 1, "", "Line"},
		{"long", func(b []byte) []byte { return append(b, ' ') }, 1, "", "Line"},
		{"BOM", func(b []byte) []byte { return append([]byte{0xef, 0xbb, 0xbf}, b...) }, 1, "", "Line"},
		{"invalid element", func(b []byte) []byte { return replaceCol(b, 2, "05x0") }, 1, "", "Element"},
		{"duplicate", func(b []byte) []byte { return bytes.Join([][]byte{b, b}, []byte{'\n'}) }, 2, ElementTemperature, "Element"},
		{"no rows", func([]byte) []byte { return nil }, 0, ElementTemperature, "Element"},
		{"missing requested", func(b []byte) []byte { return replaceCol(b, 2, "0600") }, 0, ElementTemperature, "Element"},
		{"middle empty line", func(b []byte) []byte { return bytes.Join([][]byte{b, nil, b}, []byte{'\n'}) }, 2, "", "Line"},
		{"double CR", func(b []byte) []byte { return append(b, '\r', '\r', '\n') }, 1, "", "Line"},
		{"final CR without LF", func(b []byte) []byte { return append(b, '\r') }, 1, "", "Line"},
		{"invalid Shift_JIS", func(b []byte) []byte { b[26] = 0xff; return b }, 1, ElementTemperature, "Line"},
		{"moved comma", func(b []byte) []byte { b[2], b[3] = b[3], b[2]; return b }, 1, ElementTemperature, "Line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, err := readRows(t, tc.change(bytes.Clone(row)), station)
			checkReadError(t, ds, err, "monthly", monthlyPath("49236"), tc.line, "49236", tc.element, tc.field)
		})
	}
	zero := bytes.Clone(row)
	for k := range 13 {
		zero = replaceCol(zero, 6+2*k, "     0")
		zero = replaceCol(zero, 7+2*k, "0")
	}
	for _, tc := range []struct {
		name    string
		changes map[int]string
		field   string
	}{
		{"no period nonzero years", map[int]string{3: " 1", 4: "   0", 5: "   0"}, "Years"},
		{"RMK8 without period", map[int]string{3: " 0", 4: "   0", 5: "   0", 7: "8"}, "Years"},
		{"unavailable years outside period", map[int]string{3: "30", 4: "2009"}, "Years"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bytes.Clone(zero)
			for c, v := range tc.changes {
				b = replaceCol(b, c, v)
			}
			ds, err := readRows(t, b, station)
			checkReadError(t, ds, err, "monthly", monthlyPath("49236"), 1, "49236", ElementTemperature, tc.field)
		})
	}
	for _, period := range [][3]string{{" 0", "   0", "   0"}, {"12", "1991", "2020"}} {
		b := bytes.Clone(zero)
		for k, v := range period {
			b = replaceCol(b, 3+k, v)
		}
		ds, err := readRows(t, b, station)
		if err != nil || ds.Stations[0].Normals[0].Monthly[0].Status != StatusUnavailable {
			t.Fatalf("all unavailable: dataset=%v error=%v", ds, err)
		}
	}
}

func TestReadUnrequestedRows(t *testing.T) {
	m, i := fixtureEntries(t)
	base, err := Read(memoryZIP(t, m), memoryZIP(t, i), testElements)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		change  func([]byte) []byte
		field   string
		element Element
	}{
		{"type", func(b []byte) []byte { return replaceCol(b, 0, "xx") }, "", ""},
		{"station", func(b []byte) []byte { return replaceCol(b, 1, "xxxxx") }, "", ""},
		{"value", func(b []byte) []byte { return replaceCol(b, 6, " +1e. ") }, "", ""},
		{"years", func(b []byte) []byte { return replaceCol(b, 3, "xx") }, "", ""},
		{"RMK", func(b []byte) []byte { return replaceCol(b, 7, "x") }, "", ""},
		{"Shift_JIS", func(b []byte) []byte { b[26] = 0xff; return b }, "", ""},
		{"commas", func(b []byte) []byte { b[2] = 'x'; return b }, "", ""},
		{"short", func(b []byte) []byte { return b[:142] }, "Line", ""},
		{"element", func(b []byte) []byte { return replaceCol(b, 2, "2x00") }, "Element", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mm := append([]testEntry(nil), m...)
			rows := bytes.Split(mm[2].data, []byte{'\n'})
			line := 0
			for k, row := range rows {
				if len(row) > 13 && string(row[9:13]) == "2200" {
					rows[k] = tc.change(bytes.Clone(row))
					line = k + 1
					break
				}
			}
			if line == 0 {
				t.Fatal("wind row missing")
			}
			mm[2].data = bytes.Join(rows, []byte{'\n'})
			ds, err := Read(memoryZIP(t, mm), memoryZIP(t, i), testElements)
			if tc.field != "" {
				checkReadError(t, ds, err, "monthly", monthlyPath("50066"), line, "50066", tc.element, tc.field)
				return
			}
			if err != nil || !reflect.DeepEqual(ds, base) {
				t.Fatalf("dataset=%v error=%v", ds, err)
			}
		})
	}
	row, station := firstRows(t)
	unknown := replaceCol(row, 2, "9999")
	ds, err := readRows(t, bytes.Join([][]byte{unknown, unknown, row}, []byte{'\n'}), station)
	if err != nil || len(ds.Stations[0].Normals) != 1 {
		t.Fatalf("unrequested duplicates: dataset=%v error=%v", ds, err)
	}
}
