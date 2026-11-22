package geo

import "fmt"

// Location は読み込んだ区域での所属と入力ポリゴンの境界上かを表す。
type Location struct {
	AreaCode   string
	OnBoundary bool
}

// Locate は丸めずに表せる点の所属を返す。区域外ではゼロ値を返す。
func (b *Boundaries) Locate(point Point) (Location, error) {
	if b == nil {
		return Location{}, fmt.Errorf("%w: nil Boundaries", ErrInvalidArgument)
	}
	p, err := scaledPoint(point)
	if err != nil {
		return Location{}, err
	}
	var result Location
	interior := ""
	for _, a := range b.areas {
		if !a.box.contains(p) {
			continue
		}
		inside, boundary := false, false
		for _, poly := range a.polygons {
			if !poly.box.contains(p) {
				continue
			}
			in, on := polygonLocation(poly, p)
			inside = inside || in
			boundary = boundary || on
		}
		if inside {
			if interior != "" {
				return Location{}, (ReadError{Key: a.code}).fail("Geometry", invalid("point lies inside overlapping areas %s and %s", interior, a.code))
			}
			interior = a.code
		}
		if (inside || boundary) && result.AreaCode == "" {
			result.AreaCode = a.code
		}
		result.OnBoundary = result.OnBoundary || boundary
	}
	return result, nil
}

func polygonLocation(p polygon, point intPoint) (inside, boundary bool) {
	inside, boundary = ringLocation(p.rings[0], point)
	if !inside && !boundary {
		return false, false
	}
	for _, hole := range p.rings[1:] {
		if !hole.box.contains(point) {
			continue
		}
		in, on := ringLocation(hole, point)
		if on {
			return false, true
		}
		if in {
			return false, false
		}
	}
	return inside, boundary
}

func ringLocation(r ring, p intPoint) (inside, boundary bool) {
	if !r.box.contains(p) {
		return false, false
	}
	previous := r.points[len(r.points)-1]
	for _, next := range r.points {
		if min(previous.y, next.y) <= p.y && p.y <= max(previous.y, next.y) {
			sign := cross(previous, next, p).sign()
			if sign == 0 && min(previous.x, next.x) <= p.x && p.x <= max(previous.x, next.x) {
				return false, true
			}
			if (previous.y > p.y) != (next.y > p.y) && ((next.y > previous.y && sign > 0) || (next.y < previous.y && sign < 0)) {
				inside = !inside
			}
		}
		previous = next
	}
	return inside, false
}

// AreaCodes は統合後のコードを昇順で返す。
func (b *Boundaries) AreaCodes() []string {
	result := make([]string, len(b.areas))
	for i, a := range b.areas {
		result[i] = a.code
	}
	return result
}
