# N03 のテストデータ

`n03-gunma.geojson` は国土交通省「国土数値情報（行政区域データ）」の群馬県、2026年1月1日基準の GeoJSON から切り出した。

配布ページは https://nlftp.mlit.go.jp/ksj/gml/datalist/KsjTmplt-N03-2026.html 。取得元は https://nlftp.mlit.go.jp/ksj/gml/data/N03/N03-2026/N03-20260101_10_GML.zip 。取得日時は2026年11月22日19:41 JST。元 ZIP は3,615,329バイト、SHA-256 は `0981c3412b098ff2a9947e7a8bc57bc353275c2c337b76f7adea52c08b0d7b07`。

ZIP 内の `N03-20260101_10.geojson` の FeatureCollection から、JSON の地物の文字数が小さい順に2地物を選んだ。コードは `10202`（高崎市の小さい方の地物）と `10464`（玉村町）。地物の JSON の開始位置と終了位置を求め、その範囲を文字列のままコピーした。属性と座標の数値は変更していない。FeatureCollection の features 以外の項目を残し、外側の JSON の空白と末尾の改行だけを整えた。浮動小数点数から座標を書き戻していない。

`rectangles.geojson` はテスト用に作った矩形で、実際の行政区域ではない。架空のコードに、穴のある Polygon、飛び地を持つ MultiPolygon、同じコードの別地物、共有辺、null と空文字列の所属未定地を組み合わせた。境界や内外の期待値は記載した座標から求める。精度の限界や壊れた構造はテスト内で作る。

ZIP は fixture として保存せず、通常テストが `archive/zip.Writer` でメモリ上に作る。実物全体のテストは、元 ZIP をリポジトリ外に保存し、そのディレクトリを `GEO_TEST_ZIP_DIR` に指定したときだけ動く。テストからダウンロードしない。この段階の実物テストは N03 だけを対象にする。

```sh
GEO_TEST_ZIP_DIR=/path/to/zip-directory go test ./geo -run TestReadN03FullZIP -v -count=1
```

実物テストは SHA-256、37地物・37ポリゴン・38輪・閉じ点を除く124,161頂点、小数部の最大15桁を照合する。前橋市役所付近の緯度 `36.3895`・経度 `139.0634` が `10201` に入ることと、ReadN03 の所要時間、同じ1点の Locate の所要時間も確認する。1回の Locate が時計の分解能を下回る場合に備え、10,000回の平均時間も出す。
