# climate の実物テスト

通常テストは架空の区域と地点をメモリ上で作り、通信しない。
`TestCandidatesFullZIP` は `GEO_TEST_ZIP_DIR` の5つの ZIP を使う。未設定なら Skip、設定したファイルが無ければ失敗する。テストから取得しない。

```sh
GEO_TEST_ZIP_DIR=/path/to/zip-directory go test ./climate -run '^TestCandidatesFullZIP$' -v -count=1
```

N03・人口・G04-c の出典、版、取得日時とハッシュは [geo の出典](../../geo/testdata/README.md) を参照する。N03 は ZIP 全体、人口と G04-c は読み取る本文の SHA-256 を照合する。

気象庁の出典は [平年値ダウンロード](https://www.data.jma.go.jp/stats/data/mdrr/normal/) の2020年平年値、基準期間1991〜2020年のデータ。形式と版は [jma の出典](../../jma/testdata/README.md) を参照する。

| ZIP | 取得元 | ZIP 全体の SHA-256 |
| --- | --- | --- |
| `normal_amedas_monthly.zip` | [月・年別](https://www.data.jma.go.jp/stats/data/mdrr/normal/2020/data/normal_amedas_monthly.zip) | `0c9e10f7a04df02ff94c5bd57326b8b2f089464dd0d9c9e98d3d9f3b2233a776` |
| `amedas_station_index.zip` | [地点情報](https://www.data.jma.go.jp/stats/data/mdrr/normal/2020/data/amedas_station_index.zip) | `b5f453a7520ac74aec525862bcc17fee84e4d1eef72c69c614e7d3cd4ef75db8` |

2026年11月22日21:23:13.953・21:23:14.474 JST に月・年別を、21:23:14.647・21:23:14.701 JST に地点情報を、それぞれ別の HTTP リクエストで2回取得した。月・年別は各3,131,687バイト、地点情報は各40,835バイト。両 ZIP とも2回の ZIP 全体のハッシュは一致し、ZIP 内のパスと各 CSV の展開後のハッシュもすべて一致した。今回の取得での結果であり、将来の不変性を保証しない。

月・年別は1,298個の CSV をパスの辞書順に並べ、各ファイルについてパス、NUL、本文の SHA-256 の小文字16進表記、LF を連結して SHA-256 を求めた。集約値は `003f6db30608b088b34b29c49ae05628ecf1a1dd0ed01329d940a0bccd5fd6ad`。地点情報の `amedas_station_index.csv` の本文は `ce78300364c905ffbc68462fa790afddbbb04ac6f36b181e8efdfd6d5e2abc67`。実物テストはこの中身のハッシュを照合し、ZIP 全体のハッシュをログに残す。

群馬県の N03 と5439の人口・G04-c から `10204`・`10205`・`10464` の代表点と平均標高を求め、全国の地点を対象に6要素の先頭候補をログへ出す。地点 ID、区域内か、丸め前の距離、符号つき標高差を記録する。候補の値は期待値に固定しない。入力の範囲が5439に限られるので、区域全体の被覆の確認は別途必要になる。
