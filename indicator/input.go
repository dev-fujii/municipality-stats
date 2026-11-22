package indicator

import (
	"fmt"
	"math/big"
	"regexp"

	"github.com/dev-fujii/municipality-stats/estat"
)

var (
	areaPattern   = regexp.MustCompile(`^[0-9]{5}$`)
	numberPattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
)

type recordKey struct {
	item Item
	area string
	year int
}

type inputRecord struct {
	item   Item
	year   int
	status estat.Status
	value  *big.Rat
}

func validateInput(d Definition, areas []string, records []estat.Record) (map[string][]inputRecord, error) {
	if err := ValidateDefinition(d); err != nil {
		return nil, err
	}
	wanted, err := validateAreas(areas)
	if err != nil {
		return nil, err
	}
	seen := make(map[recordKey]bool, len(records))
	parsed := make([]inputRecord, len(records))
	for i, r := range records {
		where := InputError{Index: i, Area: r.Area, Item: Item{TableID: r.TableID, Code: r.Code}, Year: r.Year}
		field, value, err := validateRecord(r)
		if err != nil {
			where.Field = fmt.Sprintf("Records[%d].%s", i, field)
			where.Err = err
			return nil, &where
		}
		key := recordKey{item: where.Item, area: r.Area, year: r.Year}
		if seen[key] {
			where.Field = fmt.Sprintf("Records[%d]", i)
			where.Err = ErrDuplicateRecord
			return nil, &where
		}
		seen[key] = true
		parsed[i] = inputRecord{item: where.Item, year: r.Year, status: r.Value.Status, value: value}
	}
	byArea := make(map[string][]inputRecord, len(areas))
	for i, r := range parsed {
		if area := records[i].Area; wanted[area] && (r.item == d.A || (d.B != nil && r.item == *d.B)) {
			byArea[area] = append(byArea[area], r)
		}
	}
	return byArea, nil
}

func validateAreas(areas []string) (map[string]bool, error) {
	if len(areas) == 0 {
		return nil, &InputError{Index: -1, Field: "Areas", Err: ErrInvalidArgument}
	}
	seen := make(map[string]bool, len(areas))
	for i, area := range areas {
		if !areaPattern.MatchString(area) || seen[area] {
			return nil, &InputError{Index: i, Area: area, Field: fmt.Sprintf("Areas[%d]", i), Err: ErrInvalidArgument}
		}
		seen[area] = true
	}
	return seen, nil
}

func validateRecord(r estat.Record) (string, *big.Rat, error) {
	if field := invalidItemField(Item{TableID: r.TableID, Code: r.Code}); field != "" {
		return field, nil, fmt.Errorf("%w: must be nonempty without surrounding whitespace", ErrInvalidRecord)
	}
	if !areaPattern.MatchString(r.Area) {
		return "Area", nil, fmt.Errorf("%w: must be five ASCII digits", ErrInvalidRecord)
	}
	if r.Year <= 0 {
		return "Year", nil, fmt.Errorf("%w: must be positive", ErrInvalidRecord)
	}
	var symbol string
	switch r.Value.Status {
	case estat.StatusOK:
		if !numberPattern.MatchString(r.Value.Raw) {
			return "Value.Raw", nil, fmt.Errorf("%w: invalid decimal %q", ErrInvalidRecord, r.Value.Raw)
		}
		value, ok := new(big.Rat).SetString(r.Value.Raw)
		if !ok {
			return "Value.Raw", nil, fmt.Errorf("%w: cannot convert decimal %q", ErrInvalidRecord, r.Value.Raw)
		}
		return "", value, nil
	case estat.StatusNotSurveyed:
		symbol = "***"
	case estat.StatusUnavailable:
		symbol = "-"
	case estat.StatusConfidential:
		symbol = "X"
	default:
		return "Value.Status", nil, fmt.Errorf("%w: unknown status %d", ErrInvalidRecord, r.Value.Status)
	}
	if r.Value.Raw != symbol {
		return "Value.Raw", nil, fmt.Errorf("%w: status requires %q", ErrInvalidRecord, symbol)
	}
	return "", nil, nil
}
