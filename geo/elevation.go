package geo

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"errors"
	"io"
	"math/big"
	"path"
	"regexp"
	"slices"
	"strings"
)

// ElevationCell は500mメッシュの平均標高を持つ。Meters が nil なら欠損。
type ElevationCell struct {
	MeshCode string
	Meters   *big.Rat
}

const (
	ksjNamespace   = "http://nlftp.mlit.go.jp/ksj/schemas/ksj-app"
	gmlNamespace   = "http://www.opengis.net/gml/3.2"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
)

var (
	elevationPath    = regexp.MustCompile(`^G04-c-11_([0-9]{4})-jgd\.xml$`)
	elevationDecimal = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+)?$`)
)

// ReadElevation は各 ZIP の平均標高を読み、メッシュコード順に返す。
func ReadElevation(sources []ZIPSource) ([]ElevationCell, error) {
	if err := validateSources(sources); err != nil {
		return nil, err
	}
	cells := make([]ElevationCell, 0)
	seen := make(map[string]bool)
	for _, src := range sources {
		data, where, err := selectedZIPBytes(src, elevationPath, "G04-c XML")
		if err != nil {
			return nil, err
		}
		p := elevationXML{
			decoder: xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))),
			where:   where,
			primary: elevationPath.FindStringSubmatch(path.Base(where.File))[1],
			seen:    seen,
		}
		if err := p.read(); err != nil {
			return nil, err
		}
		cells = append(cells, p.cells...)
	}
	slices.SortFunc(cells, func(a, b ElevationCell) int { return cmp.Compare(a.MeshCode, b.MeshCode) })
	return cells, nil
}

type elevationXML struct {
	decoder *xml.Decoder
	where   ReadError
	primary string
	seen    map[string]bool
	cells   []ElevationCell
}

func (p *elevationXML) token() (xml.Token, error) {
	token, err := p.decoder.Token()
	p.where.Line, _ = p.decoder.InputPos()
	if err != nil && err != io.EOF {
		return nil, p.where.fail("XML", errors.Join(ErrInvalidFormat, err))
	}
	return token, err
}

func (p *elevationXML) read() error {
	root := false
	for {
		token, err := p.token()
		if err == io.EOF {
			if !root {
				return p.where.fail("Dataset", invalid("Dataset missing"))
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if root || t.Name != (xml.Name{Space: ksjNamespace, Local: "Dataset"}) {
				return p.where.fail("Dataset", invalid("expected one ksj:Dataset"))
			}
			root = true
			if err := p.dataset(); err != nil {
				return err
			}
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return p.where.fail("XML", invalid("text outside Dataset"))
			}
		case xml.Directive:
			return p.where.fail("XML", invalid("XML directives are not supported"))
		}
	}
}

func (p *elevationXML) children(visit func(xml.StartElement) error) error {
	for {
		token, err := p.token()
		if err == io.EOF {
			return p.where.fail("XML", invalid("unexpected end of XML"))
		}
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if err := visit(t); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return p.where.fail("XML", invalid("unexpected text"))
			}
		}
	}
}

func (p *elevationXML) skip() error {
	if err := p.decoder.Skip(); err != nil {
		p.where.Line, _ = p.decoder.InputPos()
		return p.where.fail("XML", errors.Join(ErrInvalidFormat, err))
	}
	return nil
}

func xmlAttribute(start xml.StartElement, space, local string) string {
	for _, attr := range start.Attr {
		if attr.Name == (xml.Name{Space: space, Local: local}) {
			return attr.Value
		}
	}
	return ""
}

func (p *elevationXML) text(field string) (string, error) {
	var text strings.Builder
	for {
		token, err := p.token()
		if err != nil {
			return "", err
		}
		switch t := token.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			return text.String(), nil
		case xml.StartElement:
			return "", p.where.fail(field, invalid("nested element in text"))
		}
	}
}

func (p *elevationXML) dataset() error {
	header, features := false, 0
	err := p.children(func(start xml.StartElement) error {
		p.where.Key = ""
		switch start.Name.Local {
		case "CompositeValue":
			if header || start.Name.Space != gmlNamespace || xmlAttribute(start, gmlNamespace, "id") != "recordType" {
				return p.where.fail("recordType", invalid("expected one gml:CompositeValue with gml:id recordType"))
			}
			header = true
			return p.recordType()
		case "ElevationAndSlopeAngleFourthMesh":
			if start.Name.Space != ksjNamespace {
				return p.where.fail("Namespace", invalid("expected ksj namespace"))
			}
			features++
			return p.feature()
		default:
			return p.skip()
		}
	})
	if err != nil {
		return err
	}
	if !header || features == 0 {
		return p.where.fail("Dataset", invalid("recordType or elevation features missing"))
	}
	return nil
}

func (p *elevationXML) recordType() error {
	components := false
	err := p.children(func(start xml.StartElement) error {
		if components || start.Name != (xml.Name{Space: gmlNamespace, Local: "valueComponents"}) {
			return p.where.fail("recordType", invalid("expected one gml:valueComponents"))
		}
		components = true
		names := [...]string{"4次メッシュコード", "平均標高", "最高標高", "最低標高", "最低標高コード", "最大傾斜角度", "最大傾斜方向", "最小傾斜角度", "最小傾斜方向", "平均傾斜角度"}
		index := 0
		err := p.children(func(item xml.StartElement) error {
			kind := "QuantityList"
			if index == 0 || index == 4 || index == 6 || index == 8 {
				kind = "CategoryList"
			}
			if index >= len(names) || item.Name != (xml.Name{Space: gmlNamespace, Local: kind}) {
				return p.where.fail("recordType", invalid("unexpected column %d", index+1))
			}
			if index == 1 && xmlAttribute(item, "", "uom") != "m" {
				return p.where.fail("recordType", invalid("average elevation uom must be m"))
			}
			text, err := p.text("recordType")
			if err != nil {
				return err
			}
			if strings.TrimSpace(text) != names[index] {
				return p.where.fail("recordType", invalid("unexpected title in column %d", index+1))
			}
			index++
			return nil
		})
		if err == nil && index != len(names) {
			return p.where.fail("recordType", invalid("expected ten columns"))
		}
		return err
	})
	if err == nil && !components {
		return p.where.fail("recordType", invalid("valueComponents missing"))
	}
	return err
}

type elevationRow struct {
	cell  ElevationCell
	where ReadError
}

func (p *elevationXML) feature() error {
	// 親番号は tupleList より後に現れるため、照合まで地物内の行を保持する。
	var rows []elevationRow
	parent := ""
	ranges := false
	err := p.children(func(start xml.StartElement) error {
		switch start.Name.Local {
		case "tertiaryMeshCode":
			if parent != "" || start.Name.Space != ksjNamespace {
				return p.where.fail("tertiaryMeshCode", invalid("expected one ksj:tertiaryMeshCode"))
			}
			text, err := p.text("tertiaryMeshCode")
			if err != nil {
				return err
			}
			parent = strings.TrimSpace(text)
			if err := validateMeshCode(parent + "1"); err != nil {
				return p.where.fail("tertiaryMeshCode", err)
			}
			return nil
		case "rangeSet":
			if ranges || start.Name.Space != gmlNamespace {
				return p.where.fail("rangeSet", invalid("expected one gml:rangeSet"))
			}
			ranges = true
			return p.rangeSet(&rows)
		default:
			return p.skip()
		}
	})
	if err != nil {
		return err
	}
	if parent == "" || !ranges {
		return p.where.fail("tertiaryMeshCode", invalid("tertiaryMeshCode or rangeSet missing"))
	}
	for _, row := range rows {
		if row.cell.MeshCode[:8] != parent {
			return row.where.fail("MeshCode", invalid("mesh differs from parent tertiaryMeshCode %q", parent))
		}
		p.cells = append(p.cells, row.cell)
	}
	return nil
}

func (p *elevationXML) rangeSet(rows *[]elevationRow) error {
	blocks := 0
	err := p.children(func(start xml.StartElement) error {
		if start.Name != (xml.Name{Space: gmlNamespace, Local: "DataBlock"}) {
			return p.where.fail("DataBlock", invalid("expected gml:DataBlock"))
		}
		blocks++
		return p.dataBlock(rows)
	})
	if err == nil && blocks == 0 {
		return p.where.fail("DataBlock", invalid("DataBlock missing"))
	}
	return err
}

func (p *elevationXML) dataBlock(rows *[]elevationRow) error {
	parameters, tuples := false, false
	err := p.children(func(start xml.StartElement) error {
		switch start.Name {
		case xml.Name{Space: gmlNamespace, Local: "rangeParameters"}:
			if parameters || xmlAttribute(start, xlinkNamespace, "href") != "#recordType" {
				return p.where.fail("rangeParameters", invalid("expected one reference to #recordType"))
			}
			parameters = true
			text, err := p.text("rangeParameters")
			if err == nil && strings.TrimSpace(text) != "" {
				return p.where.fail("rangeParameters", invalid("expected an empty reference"))
			}
			return err
		case xml.Name{Space: gmlNamespace, Local: "tupleList"}:
			if tuples {
				return p.where.fail("tupleList", invalid("duplicate tupleList"))
			}
			tuples = true
			where := p.where
			text, err := p.text("tupleList")
			if err != nil {
				return err
			}
			parsed, err := p.tupleRows(text, where)
			*rows = append(*rows, parsed...)
			return err
		default:
			return p.where.fail("DataBlock", invalid("unexpected element %s", start.Name.Local))
		}
	})
	if err == nil && (!parameters || !tuples) {
		return p.where.fail("DataBlock", invalid("rangeParameters or tupleList missing"))
	}
	return err
}

func (p *elevationXML) tupleRows(text string, where ReadError) ([]elevationRow, error) {
	lines := strings.Split(text, "\n")
	first, last := 0, len(lines)
	for first < last && strings.TrimSpace(lines[first]) == "" {
		first++
	}
	for last > first && strings.TrimSpace(lines[last-1]) == "" {
		last--
	}
	if first == last {
		return nil, where.fail("tupleList", invalid("empty tupleList"))
	}
	rows := make([]elevationRow, 0, last-first)
	for i := first; i < last; i++ {
		at := where
		at.Line += i
		columns := strings.Split(lines[i], ",")
		at.Key = columns[0]
		if len(columns) != 10 {
			return nil, at.fail("tupleList", invalid("expected ten columns"))
		}
		code := columns[0]
		if err := validateMeshCode(code); err != nil {
			return nil, at.fail("MeshCode", err)
		}
		if code[:4] != p.primary || p.seen[code] {
			return nil, at.fail("MeshCode", invalid("primary mesh differs from filename or duplicate mesh"))
		}
		var meters *big.Rat
		if columns[1] != "unknown" {
			if !elevationDecimal.MatchString(columns[1]) {
				return nil, at.fail("Meters", invalid("expected a decimal elevation or unknown"))
			}
			meters, _ = new(big.Rat).SetString(columns[1])
		}
		p.seen[code] = true
		rows = append(rows, elevationRow{ElevationCell{code, meters}, at})
	}
	return rows, nil
}
