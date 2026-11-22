// Package jma は、気象庁の2020年平年値の ZIP から地点情報と月・年別の値を読む。
package jma

// Element は、月・年別平年値の要素番号を表す。
type Element string

const (
	ElementTemperature      Element = "0500"
	ElementDailyMaximum     Element = "0600"
	ElementDailyMinimum     Element = "0700"
	ElementSunshineDuration Element = "3500"
	ElementMaximumSnowDepth Element = "6200"
	ElementSnowfallTotal    Element = "6300"
)

// Unit は、整数で保持する平年値の単位を表す。
type Unit string

const (
	UnitDeciCelsius Unit = "0.1℃"
	UnitDeciHour    Unit = "0.1時間"
	UnitCentimeter  Unit = "1cm"
)

// Status は、未設定、数値あり、値を得られない状態を区別する。
type Status int

const (
	StatusUnset Status = iota
	StatusOK
	StatusUnavailable
)

// Value は、元の単位の整数、RMK と値の状態を持つ。
// StatusUnavailable の Integer は数値として使わない。
type Value struct {
	Integer int64
	RMK     int
	Status  Status
}

// Normal は、1要素の資料年数、統計期間、1〜12月と年の平年値を持つ。
type Normal struct {
	Element   Element
	Unit      Unit
	Years     int
	StartYear int
	EndYear   int
	Monthly   [12]Value
	Annual    Value
}

func elementUnit(e Element) (Unit, bool) {
	switch e {
	case ElementTemperature, ElementDailyMaximum, ElementDailyMinimum:
		return UnitDeciCelsius, true
	case ElementSunshineDuration:
		return UnitDeciHour, true
	case ElementMaximumSnowDepth, ElementSnowfallTotal:
		return UnitCentimeter, true
	default:
		return "", false
	}
}
