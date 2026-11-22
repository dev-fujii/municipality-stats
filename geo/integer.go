package geo

import (
	"math/big"
	"math/bits"
)

type int128 struct {
	hi, lo   uint64
	negative bool
}

func product(a, b int64) int128 {
	negative := (a < 0) != (b < 0)
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	return int128{hi, lo, negative && (hi != 0 || lo != 0)}
}

func (a int128) sub(b int128) int128 {
	if a.negative != b.negative {
		lo, carry := bits.Add64(a.lo, b.lo, 0)
		hi, _ := bits.Add64(a.hi, b.hi, carry)
		return int128{hi, lo, a.negative}
	}
	if a.hi < b.hi || (a.hi == b.hi && a.lo < b.lo) {
		a, b = b, a
		a.negative = !a.negative
	}
	lo, borrow := bits.Sub64(a.lo, b.lo, 0)
	hi, _ := bits.Sub64(a.hi, b.hi, borrow)
	return int128{hi, lo, a.negative && (hi != 0 || lo != 0)}
}

func (a int128) sign() int {
	if a.hi == 0 && a.lo == 0 {
		return 0
	}
	if a.negative {
		return -1
	}
	return 1
}

func (a int128) bigInt(dst *big.Int) {
	var low big.Int
	dst.SetUint64(a.hi)
	dst.Lsh(dst, 64)
	low.SetUint64(a.lo)
	dst.Add(dst, &low)
	if a.negative {
		dst.Neg(dst)
	}
}

func cross(a, b, p intPoint) int128 {
	return product(b.x-a.x, p.y-a.y).sub(product(b.y-a.y, p.x-a.x))
}

type rectangle struct{ minX, minY, maxX, maxY int64 }

func (r rectangle) contains(p intPoint) bool {
	return r.minX <= p.x && p.x <= r.maxX && r.minY <= p.y && p.y <= r.maxY
}

func (r rectangle) include(s rectangle) rectangle {
	return rectangle{min(r.minX, s.minX), min(r.minY, s.minY), max(r.maxX, s.maxX), max(r.maxY, s.maxY)}
}

func (r rectangle) overlaps(s rectangle) bool {
	return r.minX < s.maxX && s.minX < r.maxX && r.minY < s.maxY && s.minY < r.maxY
}
