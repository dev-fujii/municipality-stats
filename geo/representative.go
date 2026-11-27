package geo

import (
	"cmp"
	"fmt"
	"math/big"
	"slices"
)

type RepresentativeState string

const (
	RepresentativeOK           RepresentativeState = "ok"
	RepresentativeNoPopulation RepresentativeState = "no_population"
)

type ElevationState string

const (
	ElevationOK           ElevationState = "ok"
	ElevationPartial      ElevationState = "partial_mesh"
	ElevationMissing      ElevationState = "missing_mesh"
	ElevationNoPopulation ElevationState = "no_population"
)

// Representative は人口で重み付けした代表点と平均標高を持つ。
type Representative struct {
	AreaCode                    string
	Point                       *Point
	Population                  *big.Int
	ElevationMeters             *big.Rat
	State                       RepresentativeState
	ElevationState              ElevationState
	MissingElevationMeshes      []string
	ElevationPopulationCoverage *big.Rat
	ElevationCellCoverage       *big.Rat
}

type RepresentativeOptions struct {
	AllowPartialElevation bool
}

// Representatives は対象の区域の代表点と平均標高をコードの昇順で返す。
// 人口の正のセルに標高欠損があれば、残りだけで標高を平均しない。
func Representatives(b *Boundaries, population []PopulationCell, elevation []ElevationCell, areaCodes []string) ([]Representative, error) {
	return RepresentativesWithOptions(b, population, elevation, areaCodes, RepresentativeOptions{})
}

// RepresentativesWithOptions は欠けた標高を除いて平均するかを opts で選べる。
// 取得率は人口の正のセルを対象に、人口の重みとセル数でそれぞれ求める。
// 標高が全部あれば1、全部欠ければ0、人口が無ければnilを返す。
func RepresentativesWithOptions(b *Boundaries, population []PopulationCell, elevation []ElevationCell, areaCodes []string, opts RepresentativeOptions) ([]Representative, error) {
	if b == nil || len(areaCodes) == 0 {
		return nil, fmt.Errorf("%w: nil Boundaries or empty areaCodes", ErrInvalidArgument)
	}
	available := make(map[string]bool, len(b.areas))
	for _, a := range b.areas {
		available[a.code] = true
	}
	totals := make(map[string]*representativeTotal, len(areaCodes))
	for _, code := range areaCodes {
		if !areaCode(code) {
			return nil, fmt.Errorf("%w: invalid area code %q", ErrInvalidArgument, code)
		}
		where := ReadError{Key: code}
		if !available[code] {
			return nil, where.fail("AreaCode", invalid("area not found in Boundaries"))
		}
		if totals[code] != nil {
			return nil, where.fail("AreaCode", invalid("duplicate area code"))
		}
		totals[code] = new(representativeTotal)
	}
	if err := validatePopulationCells(population); err != nil {
		return nil, err
	}
	heights, err := elevationByMesh(elevation)
	if err != nil {
		return nil, err
	}
	cells := slices.Clone(population)
	slices.SortFunc(cells, func(a, b PopulationCell) int { return cmp.Compare(a.MeshCode, b.MeshCode) })
	for _, cell := range cells {
		if cell.People == 0 {
			continue
		}
		center, err := MeshCenter(cell.MeshCode)
		if err != nil {
			return nil, err
		}
		location, err := b.Locate(center)
		if err != nil {
			return nil, err
		}
		if total := totals[location.AreaCode]; total != nil {
			total.add(cell, center, heights[cell.MeshCode])
		}
	}
	codes := slices.Clone(areaCodes)
	slices.Sort(codes)
	result := make([]Representative, 0, len(codes))
	for _, code := range codes {
		result = append(result, totals[code].representative(code, opts))
	}
	return result, nil
}

func validatePopulationCells(cells []PopulationCell) error {
	seen := make(map[string]bool, len(cells))
	for _, cell := range cells {
		where := ReadError{Key: cell.MeshCode}
		if err := validateMeshCode(cell.MeshCode); err != nil {
			return where.fail("MeshCode", err)
		}
		if seen[cell.MeshCode] {
			return where.fail("MeshCode", invalid("duplicate population mesh"))
		}
		if cell.People < 0 {
			return where.fail("People", invalid("negative population"))
		}
		seen[cell.MeshCode] = true
	}
	return nil
}

func elevationByMesh(cells []ElevationCell) (map[string]*big.Rat, error) {
	heights := make(map[string]*big.Rat, len(cells))
	for _, cell := range cells {
		where := ReadError{Key: cell.MeshCode}
		if err := validateMeshCode(cell.MeshCode); err != nil {
			return nil, where.fail("MeshCode", err)
		}
		if _, exists := heights[cell.MeshCode]; exists {
			return nil, where.fail("MeshCode", invalid("duplicate elevation mesh"))
		}
		heights[cell.MeshCode] = nil
		if cell.Meters != nil {
			heights[cell.MeshCode] = new(big.Rat).Set(cell.Meters)
		}
	}
	return heights, nil
}

type representativeTotal struct {
	population                     big.Int
	elevationPopulation            big.Int
	cells, elevationCells          int64
	latitude, longitude, elevation big.Rat
	missing                        []string
}

func (s *representativeTotal) add(cell PopulationCell, center Point, meters *big.Rat) {
	people := big.NewInt(cell.People)
	s.population.Add(&s.population, people)
	s.cells++
	weight := new(big.Rat).SetInt(people)
	var term big.Rat
	s.latitude.Add(&s.latitude, term.Mul(weight, center.Latitude))
	s.longitude.Add(&s.longitude, term.Mul(weight, center.Longitude))
	if meters == nil {
		s.missing = append(s.missing, cell.MeshCode)
	} else {
		s.elevationPopulation.Add(&s.elevationPopulation, people)
		s.elevationCells++
		s.elevation.Add(&s.elevation, term.Mul(weight, meters))
	}
}

func (s *representativeTotal) representative(code string, opts RepresentativeOptions) Representative {
	r := Representative{
		AreaCode: code, Population: new(big.Int).Set(&s.population),
		State: RepresentativeNoPopulation, ElevationState: ElevationNoPopulation,
	}
	if s.population.Sign() == 0 {
		return r
	}
	weight := new(big.Rat).SetInt(&s.population)
	r.ElevationPopulationCoverage = new(big.Rat).SetFrac(&s.elevationPopulation, &s.population)
	r.ElevationCellCoverage = big.NewRat(s.elevationCells, s.cells)
	r.State = RepresentativeOK
	r.Point = &Point{
		Latitude:  new(big.Rat).Quo(&s.latitude, weight),
		Longitude: new(big.Rat).Quo(&s.longitude, weight),
	}
	if len(s.missing) > 0 {
		r.ElevationState = ElevationMissing
		r.MissingElevationMeshes = slices.Clone(s.missing)
		if opts.AllowPartialElevation && s.elevationPopulation.Sign() > 0 {
			r.ElevationState = ElevationPartial
			r.ElevationMeters = new(big.Rat).Quo(&s.elevation, new(big.Rat).SetInt(&s.elevationPopulation))
		}
	} else {
		r.ElevationState = ElevationOK
		r.ElevationMeters = new(big.Rat).Quo(&s.elevation, weight)
	}
	return r
}
