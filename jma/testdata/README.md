# 検証用の平年値

出典は気象庁の [平年値ダウンロード](https://www.data.jma.go.jp/stats/data/mdrr/normal/)。2020年平年値の第5版（2025-03-25）、基準の統計期間は1991〜2020年。取得日は2026-11-15。第5版以降の更新がないことを2026-11-22に気象庁のページで確認済み。

| 元の ZIP | SHA-256 |
| --- | --- |
| [normal_amedas_monthly.zip](https://www.data.jma.go.jp/stats/data/mdrr/normal/2020/data/normal_amedas_monthly.zip) | `0c9e10f7a04df02ff94c5bd57326b8b2f089464dd0d9c9e98d3d9f3b2233a776` |
| [amedas_station_index.zip](https://www.data.jma.go.jp/stats/data/mdrr/normal/2020/data/amedas_station_index.zip) | `b5f453a7520ac74aec525862bcc17fee84e4d1eef72c69c614e7d3cd4ef75db8` |

月・年別は `monthly/area49/nml_amd_m_49236.csv`、`monthly/area49/nml_amd_m_49251.csv`、`monthly/area50/nml_amd_m_50066.csv`、`monthly/area50/nml_amd_m_50477.csv` の全122行を、それぞれ ZIP 内のバイト列のまま取り出した。配置先ではファイル名だけを残している。

地点情報は `amedas_station_index.csv` の表題2行と、観測所番号 `49236`、`49251`、`50066`、`50477` の4行を元の順で連結した。各行のバイト列は変更していない。

どちらも Shift_JIS、LF、最終改行あり。空白の埋め草と半角カナを保っている。壊れた入力と ZIP はテスト中に作る。
