package geo

import (
	"math/big"
	"math/rand/v2"
	"testing"
)

func referenceCross(a, b, p intPoint) *big.Int {
	left := new(big.Int).Mul(big.NewInt(b.x-a.x), big.NewInt(p.y-a.y))
	right := new(big.Int).Mul(big.NewInt(b.y-a.y), big.NewInt(p.x-a.x))
	return left.Sub(left, right)
}

func TestCross(t *testing.T) {
	worldX, worldY := 180*coordinateScale, 90*coordinateScale
	for _, tt := range []struct {
		name    string
		a, b, p intPoint
		sign    int
	}{
		{"positive", intPoint{}, intPoint{2, 0}, intPoint{1, 1}, 1},
		{"negative", intPoint{}, intPoint{2, 0}, intPoint{1, -1}, -1},
		{"zero", intPoint{}, intPoint{2, 2}, intPoint{3, 3}, 0},
		{"beyond int64 product", intPoint{}, intPoint{worldX, 0}, intPoint{0, worldY}, 1},
		{"world difference", intPoint{-worldX, -worldY}, intPoint{worldX, worldY}, intPoint{worldX, -worldY}, -1},
		{"cancellation", intPoint{-worldX, -worldY}, intPoint{worldX, worldY}, intPoint{worldX - 2, worldY - 1}, 0},
		{"one unit", intPoint{-worldX, -worldY}, intPoint{worldX, worldY}, intPoint{worldX - 1, worldY}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := cross(tt.a, tt.b, tt.p)
			var value big.Int
			got.bigInt(&value)
			want := referenceCross(tt.a, tt.b, tt.p)
			if value.Cmp(want) != 0 || got.sign() != tt.sign {
				t.Fatalf("got=%s sign=%d want=%s sign=%d", &value, got.sign(), want, tt.sign)
			}
		})
	}
}

func TestCrossReference(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	point := func() intPoint {
		return intPoint{random.Int64N(360*coordinateScale+1) - 180*coordinateScale, random.Int64N(180*coordinateScale+1) - 90*coordinateScale}
	}
	for i := 0; i < 10000; i++ {
		a, b, p := point(), point(), point()
		got := cross(a, b, p)
		var value big.Int
		got.bigInt(&value)
		want := referenceCross(a, b, p)
		if value.Cmp(want) != 0 || got.sign() != want.Sign() {
			t.Fatalf("a=%v b=%v p=%v got=%s want=%s", a, b, p, &value, want)
		}
	}
}
