# N03 のテストデータ

`n03-gunma.geojson` は国土交通省「国土数値情報（行政区域データ）」の群馬県、2026年1月1日基準の GeoJSON から切り出した。

配布ページは https://nlftp.mlit.go.jp/ksj/gml/datalist/KsjTmplt-N03-2026.html 。取得元は https://nlftp.mlit.go.jp/ksj/gml/data/N03/N03-2026/N03-20260101_10_GML.zip 。取得日時は2026年11月22日19:41 JST。元 ZIP は3,615,329バイト、SHA-256 は `0981c3412b098ff2a9947e7a8bc57bc353275c2c337b76f7adea52c08b0d7b07`。

ZIP 内の `N03-20260101_10.geojson` の FeatureCollection から、JSON の地物の文字数が小さい順に2地物を選んだ。コードは `10202`（高崎市の小さい方の地物）と `10464`（玉村町）。地物の JSON の開始位置と終了位置を求め、その範囲を文字列のままコピーした。属性と座標の数値は変更していない。FeatureCollection の features 以外の項目を残し、外側の JSON の空白と末尾の改行だけを整えた。浮動小数点数から座標を書き戻していない。

`rectangles.geojson` はテスト用に作った矩形で、実際の行政区域ではない。架空のコードに、穴のある Polygon、飛び地を持つ MultiPolygon、同じコードの別地物、共有辺、null と空文字列の所属未定地を組み合わせた。境界や内外の期待値は記載した座標から求める。精度の限界や壊れた構造はテスト内で作る。

ZIP は fixture として保存せず、通常テストが `archive/zip.Writer` でメモリ上に作る。実物全体のテストは、元 ZIP をリポジトリ外に保存し、そのディレクトリを `GEO_TEST_ZIP_DIR` に指定したときだけ動く。テストからダウンロードしない。

```sh
GEO_TEST_ZIP_DIR=/path/to/zip-directory go test ./geo -run 'TestRead(N03|Population)FullZIP' -v -count=1
```

実物テストは SHA-256、37地物・37ポリゴン・38輪・閉じ点を除く124,161頂点、小数部の最大15桁を照合する。前橋市役所付近の緯度 `36.3895`・経度 `139.0634` が `10201` に入ることと、ReadN03 の所要時間、同じ1点の Locate の所要時間も確認する。1回の Locate が時計の分解能を下回る場合に備え、10,000回の平均時間も出す。

# 人口のテストデータ

出典: 総務省統計局「令和2年国勢調査に関する地域メッシュ統計」（政府統計の総合窓口 e-Stat）。500mメッシュ、統計表 `T001141`、1次メッシュ `5439`、JGD2011 を使用した。[配布ページ](https://www.e-stat.go.jp/gis/statmap-search?type=1&toukeiCode=00200521&toukeiYear=2020&aggregateUnit=H&serveyId=H002005112020&statsId=T001141&datum=2011&meshCode=5439&download_disp_flg=1)、[取得元 ZIP](https://www.e-stat.go.jp/gis/statmap-search/data?statsId=T001141&code=5439&downloadType=2)。

`tblT001141H5439.txt` は、上の統計データを municipality-stats のテスト用に切り出して作成した。[e-Stat 利用規約](https://www.e-stat.go.jp/terms-of-use)と[統計地理情報システム機能利用規約](https://www.e-stat.go.jp/gis-terms)に従い、出典と加工内容を示す。

取得日時は2026年11月22日19:54:57 JST。元 ZIP は897,729バイトで、SHA-256 は `73b15df636c53d6e27d80c6def25c582814800c9403df811183e43a5da54196f`。ZIP 内の本文は2,761,605バイト、SHA-256 は `32d3bb761e2dd13c0be08a850ee9f7820c3f4eb7b4f2ca11f81a17de0b93cd5a`。

設計3.1に記録された ZIP の SHA-256 `3163b3b217d22422cff3b631b5a0fe91119a2f54113ae60463fdc63109debe87` とは異なる。19:56:41 JST の再取得でも ZIP の SHA-256 は `b68e6a0c54f54cbc955c6ecff74e87a964775abe60249fac2ef9548b5fa93054` に変わったが、本文の SHA-256 は一致した。この2回では ZIP 内のファイル時刻が変わり、ZIP 全体で7バイトの差があった。設計の記録は変更していない。実物テストは設計の ZIP と19:54取得分の ZIP のハッシュを受け付け、本文のハッシュも検査する。未記録の ZIP は失敗とし、内容と出典を確認してから期待値を更新する。

ZIP 内の `tblT001141H5439.txt` を改行込みのバイト列に分け、表題の1〜2行目と、HTKSYORI が `0`・`1`・`2` で最初に現れるデータ行を元の順で連結した。切り出した物理行番号は3・12・13、メッシュコードは `543900013`・`543900041`・`543900042`、総人口はそれぞれ13・22・10人。表題2行、全54列、Shift_JIS、全角空白、秘匿等の印、CRLF、末尾の改行を含め、残した行のバイトは変更していない。fixture の SHA-256 は `458f1b3e4099da532d9bf8d5e5f7a2a57e02ef40f5a1bdca602a1533599fc7fa`。`.gitattributes` で人口 txt の改行変換を無効にしている。

人口0、記号、壊れた表題・列数・文字コードや ZIP はテスト内で作る。人口の正の行でも、HTKSYORI・HTKSAKI・GASSAN や他の49項目によって人口総数を移し替えないことを確かめる。実物テストは表題を除く18,334行、HTKSYORI の `0` が15,392行・`1` が1,342行・`2` が1,600行、全行の総人口が正であることを設計3.1と照合する。今回計測した人口の合計5,018,881人と返り値の合計も照合し、`ReadPopulation` の所要時間を出す。
