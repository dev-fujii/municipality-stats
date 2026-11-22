package jma

import (
	"archive/zip"
	"fmt"
)

var monthlyWidths = []int{2, 5, 4, 2, 4, 4, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1}

func readNormals(f *zip.File, id string, wanted map[Element]bool, ordered []Element) ([]Normal, error) {
	where := ReadError{Archive: "monthly", File: f.Name, StationID: id}
	data, err := fileBytes(f, where)
	if err != nil {
		return nil, err
	}
	normals := make(map[Element]Normal, len(wanted))
	err = eachLine(data, func(line int, row []byte) error {
		at := where
		at.Line = line
		if len(row) != 143 {
			return at.fail("Line", invalid("line length %d, want 143", len(row)))
		}
		if !digitsPattern.Match(row[9:13]) {
			return at.fail("Element", invalid("element must be four ASCII digits"))
		}
		at.Element = Element(row[9:13])
		if !wanted[at.Element] {
			return nil
		}
		if _, exists := normals[at.Element]; exists {
			return at.fail("Element", invalid("duplicate element"))
		}
		n, err := parseNormal(row, at)
		if err != nil {
			return err
		}
		normals[at.Element] = n
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]Normal, 0, len(ordered))
	for _, element := range ordered {
		n, ok := normals[element]
		if !ok {
			where.Element = element
			return nil, where.fail("Element", invalid("requested element missing"))
		}
		result = append(result, n)
	}
	return result, nil
}

func parseNormal(row []byte, where ReadError) (Normal, error) {
	var n Normal
	cols, err := columns(row, 143, monthlyWidths)
	if err != nil {
		return n, where.fail("Line", err)
	}
	if _, err := decodeSJIS(row); err != nil {
		return n, where.fail("Line", err)
	}
	if string(cols[0]) != "21" {
		return n, where.fail("Line", invalid("normal type must be 21"))
	}
	if !digitsPattern.Match(cols[1]) || string(cols[1]) != where.StationID {
		return n, where.fail("StationID", invalid("station number differs from file name"))
	}
	n.Element = where.Element
	n.Unit, _ = elementUnit(n.Element)
	for i, field := range []struct {
		name string
		dest *int
	}{{"Years", &n.Years}, {"StartYear", &n.StartYear}, {"EndYear", &n.EndYear}} {
		x, err := parseInteger(string(cols[i+3]), false, true)
		if err != nil {
			return n, where.fail(field.name, err)
		}
		*field.dest = int(x)
	}
	if n.Years > 30 {
		return n, where.fail("Years", invalid("years outside 0..30"))
	}
	if n.StartYear == 0 && n.EndYear == 0 {
		if n.Years != 0 {
			return n, where.fail("Years", invalid("nonzero years without statistical period"))
		}
	} else {
		if n.StartYear < 1991 || n.StartYear > 2020 {
			return n, where.fail("StartYear", invalid("start year outside 1991..2020"))
		}
		if n.EndYear < n.StartYear || n.EndYear > 2020 {
			return n, where.fail("EndYear", invalid("invalid statistical end year"))
		}
		if n.Years < 1 || n.Years > n.EndYear-n.StartYear+1 {
			return n, where.fail("Years", invalid("years outside statistical period"))
		}
	}
	for k := range 13 {
		field := fmt.Sprintf("Monthly[%d]", k)
		if k == 12 {
			field = "Annual"
		}
		x, err := parseInteger(string(cols[6+2*k]), true, true)
		if err != nil {
			return n, where.fail(field+".Integer", err)
		}
		v := Value{Integer: x}
		switch string(cols[7+2*k]) {
		case "0":
			v.RMK, v.Status = 0, StatusUnavailable
			if x != 0 {
				return n, where.fail(field+".Integer", invalid("RMK 0 requires integer 0"))
			}
		case "8":
			v.RMK, v.Status = 8, StatusOK
			if n.Years == 0 || n.StartYear == 0 || n.EndYear == 0 {
				return n, where.fail("Years", invalid("RMK 8 requires years and statistical period"))
			}
		default:
			return n, where.fail(field+".RMK", invalid("RMK must be 0 or 8"))
		}
		if k == 12 {
			n.Annual = v
		} else {
			n.Monthly[k] = v
		}
	}
	return n, nil
}
