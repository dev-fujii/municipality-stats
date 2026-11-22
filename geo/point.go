package geo

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const decimalScale int64 = 1_000_000_000_000_000
const coordinateScale int64 = 3 * decimalScale

// Point は度単位の正確な緯度経度を持つ。
type Point struct {
	Latitude  *big.Rat
	Longitude *big.Rat
}

type intPoint struct{ x, y int64 }

func scaledPoint(p Point) (intPoint, error) {
	var result intPoint
	for _, v := range []struct {
		r     *big.Rat
		limit int64
		dst   *int64
	}{{p.Longitude, 180, &result.x}, {p.Latitude, 90, &result.y}} {
		if v.r == nil || v.r.Cmp(big.NewRat(-v.limit, 1)) < 0 || v.r.Cmp(big.NewRat(v.limit, 1)) > 0 {
			return intPoint{}, fmt.Errorf("%w: nil or out-of-range coordinate", ErrInvalidArgument)
		}
		var n, q, rem big.Int
		n.Mul(v.r.Num(), big.NewInt(coordinateScale))
		q.QuoRem(&n, v.r.Denom(), &rem)
		if rem.Sign() != 0 {
			return intPoint{}, fmt.Errorf("%w: coordinate cannot be expressed in 1/(3*10^15) degrees", ErrInvalidArgument)
		}
		*v.dst = q.Int64()
	}
	return result, nil
}

func decimalCoordinate(n json.Number) (int64, error) {
	s := n.String()
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	exponent := int64(0)
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		var err error
		exponent, err = strconv.ParseInt(s[i+1:], 10, 32)
		if err != nil {
			return 0, errors.Join(invalid("coordinate exponent"), err)
		}
		s = s[:i]
	}
	whole, fraction, _ := strings.Cut(s, ".")
	fraction = strings.TrimRight(fraction, "0")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return 0, nil
	}
	shift := 15 + exponent - int64(len(fraction))
	// 指数表記の末尾の0も、固定小数点で必要な精度に数えない。
	for strings.HasSuffix(digits, "0") {
		digits = digits[:len(digits)-1]
		shift++
	}
	if shift < 0 {
		return 0, invalid("coordinate requires more than 15 decimal places")
	}
	if int64(len(digits))+shift > 19 {
		return 0, invalid("coordinate overflows int64")
	}
	digits += strings.Repeat("0", int(shift))
	if negative {
		digits = "-" + digits
	}
	v, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, errors.Join(invalid("coordinate overflows int64"), err)
	}
	return v, nil
}
