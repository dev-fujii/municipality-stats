// Package geo は行政区域、メッシュと人口加重代表点を扱う。
// Representatives は標高の欠けた区域の平均標高を返さない。
// RepresentativesWithOptions の AllowPartialElevation を有効にすると、
// 標高のある人口セルだけの人口加重平均を partial_mesh として返す。
// 標高が全部欠ける場合は missing_mesh、人口が無い場合は no_population になる。
// ElevationPopulationCoverage と ElevationCellCoverage は人口とセル数それぞれの
// 標高取得率で、全部そろえば1、全部欠ければ0、人口が無ければnilになる。
package geo
