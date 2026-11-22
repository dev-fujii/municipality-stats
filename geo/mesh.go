package geo

import (
	"fmt"
	"slices"
)

// PrimaryMeshes は区域と面積を持って交差する1次メッシュを昇順で返す。
func (b *Boundaries) PrimaryMeshes() []string { return slices.Clone(b.meshes) }

func primaryMeshes(areas []area) []string {
	seen := make(map[string]bool)
	const height = 2 * decimalScale
	for _, a := range areas {
		for _, p := range a.polygons {
			for lat := max(int64(0), p.box.minY/height); lat <= min(int64(99), p.box.maxY/height); lat++ {
				for lon := max(int64(0), p.box.minX/coordinateScale-100); lon <= min(int64(99), p.box.maxX/coordinateScale-100); lon++ {
					code := fmt.Sprintf("%02d%02d", lat, lon)
					if seen[code] {
						continue
					}
					r := rectangle{(lon + 100) * coordinateScale, lat * height, (lon + 101) * coordinateScale, (lat + 1) * height}
					if p.box.overlaps(r) && polygonIntersectsOpenRectangle(p, r) {
						seen[code] = true
					}
				}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for code := range seen {
		result = append(result, code)
	}
	slices.Sort(result)
	return result
}

func polygonIntersectsOpenRectangle(p polygon, r rectangle) bool {
	// 輪が枠の内部を通れば、その片側に面積がある。通らなければ枠内の内外は一定になる。
	for _, ring := range p.rings {
		if !ring.box.overlaps(r) {
			continue
		}
		previous := ring.points[len(ring.points)-1]
		for _, next := range ring.points {
			if segmentIntersectsOpenRectangle(previous, next, r) {
				return true
			}
			previous = next
		}
	}
	inside, _ := polygonLocation(p, intPoint{(r.minX + r.maxX) / 2, (r.minY + r.maxY) / 2})
	return inside
}

func segmentIntersectsOpenRectangle(a, b intPoint, r rectangle) bool {
	if a == b || max(a.x, b.x) <= r.minX || min(a.x, b.x) >= r.maxX || max(a.y, b.y) <= r.minY || min(a.y, b.y) >= r.maxY {
		return false
	}
	// 矩形の軸と線分の法線を分離軸にする。0だけの接触は面積に含めない。
	positive, negative := false, false
	for _, corner := range [...]intPoint{{r.minX, r.minY}, {r.maxX, r.minY}, {r.maxX, r.maxY}, {r.minX, r.maxY}} {
		sign := cross(a, b, corner).sign()
		positive = positive || sign > 0
		negative = negative || sign < 0
	}
	return positive && negative
}
