package geo

import (
	"encoding/json"
	"errors"
	"math/big"
	"testing"
)

func TestScaledPoint(t *testing.T) {
	for _, tt := range []struct {
		name  string
		point Point
		want  intPoint
	}{
		{"zero", rationalPoint("0", "0"), intPoint{}},
		{"world northeast", rationalPoint("90", "180"), intPoint{180 * coordinateScale, 90 * coordinateScale}},
		{"world southwest", rationalPoint("-90", "-180"), intPoint{-180 * coordinateScale, -90 * coordinateScale}},
		{"JMA 600", rationalPoint("21601/600", "83401/600"), intPoint{83401 * (coordinateScale / 600), 21601 * (coordinateScale / 600)}},
		{"mesh 480 and 320", rationalPoint("17281/480", "44481/320"), intPoint{44481 * (coordinateScale / 320), 17281 * (coordinateScale / 480)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scaledPoint(tt.point)
			if err != nil || got != tt.want {
				t.Fatalf("got=%v err=%v want=%v", got, err, tt.want)
			}
		})
	}
}

func TestDecimalCoordinate(t *testing.T) {
	for _, tt := range []struct {
		raw     string
		want    int64
		invalid bool
	}{
		{"0", 0, false}, {"-0.000", 0, false}, {"180", 180 * decimalScale, false},
		{"-139.123456789012345", -139123456789012345, false},
		{"0.000000000000001", 1, false}, {"1e-15", 1, false},
		{"1.0000000000000000", decimalScale, false}, {"10e-16", 1, false},
		{"1391234567890123450e-16", 139123456789012345, false},
		{"1.39e2", 139 * decimalScale, false}, {"0e100", 0, false},
		{"1e-16", 0, true}, {"1.0000000000000001", 0, true},
		{"1e100", 0, true}, {"1e99999999999999999", 0, true}, {"9223.372036854775808", 0, true},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := decimalCoordinate(json.Number(tt.raw))
			if got != tt.want || (tt.invalid && !errors.Is(err, ErrInvalidFormat)) || (!tt.invalid && err != nil) {
				t.Fatalf("got=%d err=%v want=%d invalid=%v", got, err, tt.want, tt.invalid)
			}
		})
	}
	// 1回の換算以外では有理数を辺の演算に持ち込まない。
	p := Point{big.NewRat(1, 7), big.NewRat(139, 1)}
	if _, err := scaledPoint(p); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
}
