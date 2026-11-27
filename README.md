# municipality-stats

e-Stat の統計と気象庁の平年値を取り込み、市区町村ごとの値にまとめて指標を計算する Go のライブラリを作ります。
静的な HTML に書き出す見本のコマンドと `example/` も置く予定です。

設計を書き終え、e-Stat を読む `estat` パッケージから作り始める段階です。まず複数の県で試します。設計は [基本設計](docs/基本設計.md) と [詳細設計](docs/詳細設計.md) にあります。

## 手元での確認

リポジトリのルートで実行します。

```sh
go vet ./...
golangci-lint run
go test ./...
```

データの出典は e-Stat と気象庁です。出典の正式な書き方はあとで足します。

ライセンスは MIT です。詳しくは [LICENSE](LICENSE) を参照してください。

`geo.RepresentativesWithOptions` は `RepresentativeOptions{AllowPartialElevation: true}` で、標高のある人口セルだけの人口加重平均を返します。一部が欠ける状態は `partial_mesh`、全部欠ける状態は `missing_mesh` です。従来の `Representatives` は欠損があれば平均標高を返しません。`ElevationPopulationCoverage` は標高のあるセルの人口の割合、`ElevationCellCoverage` は標高のある人口セル数の割合です。全部そろえば1、全部欠ければ0、人口が無ければnilです。
