package jma

import (
	"errors"
	"strconv"
	"testing"
)

func TestReadError(t *testing.T) {
	_, cause := strconv.ParseInt("bad", 10, 64)
	err := (ReadError{Archive: "monthly", File: monthlyPath("49236"), Line: 7, StationID: "49236", Element: ElementTemperature}).fail("Years", errors.Join(ErrInvalidFormat, cause))
	checkReadError(t, nil, err, "monthly", monthlyPath("49236"), 7, "49236", ElementTemperature, "Years")
	var number *strconv.NumError
	if !errors.As(err, &number) || !errors.Is(err, cause) {
		t.Fatalf("lost number error: %v", err)
	}
	var re *ReadError
	if !errors.As(err, &re) || re.Unwrap() != re.Err {
		t.Fatalf("Unwrap=%v", err)
	}
}
