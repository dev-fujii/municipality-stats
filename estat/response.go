package estat

import (
	"encoding/json"
	"fmt"
)

// oneOrMany は、1件だけのときに配列でなくオブジェクト1つで返る VALUE と NOTE を読む。
type oneOrMany[T any] []T

// UnmarshalJSON は、次のどちらの形でも並びとして読む。
//
//	2件以上: { "VALUE": [ { "$": "102.55" }, { "$": "98.10" } ] }
//	1件:     { "VALUE": { "$": "102.55" } }
func (m *oneOrMany[T]) UnmarshalJSON(data []byte) error {
	if data[0] == '[' {
		// *m に直接読むと、この UnmarshalJSON がまた呼ばれて止まらない。
		var many []T

		if err := json.Unmarshal(data, &many); err != nil {
			return err
		}

		*m = many
		return nil
	}

	if data[0] == '{' {
		var one T
		if err := json.Unmarshal(data, &one); err != nil {
			return err
		}

		*m = oneOrMany[T]{one}
		return nil
	}

	return fmt.Errorf("e-Stat: expected array or object, got %q", data[0])
}

// noteJSON は NOTE の1件を読む。
type noteJSON struct {
	Char string `json:"@char"`
	Text string `json:"$"`
}

// valueJSON は VALUE の1件を読む。
type valueJSON struct {
	Tab   string
	Cat01 string
	Area  string
	Time  string
	Unit  string
	Raw   string
}

// UnmarshalJSON は、知らないキーと、@unit のほかのキーの欠け・空・null をエラーにする。
// タグで読むと、欠けや空は空の値のまま通り、キーの大文字と小文字の違いも通ってしまう。
func (v *valueJSON) UnmarshalJSON(data []byte) error {
	// null を nil として見分けるため、値は *string で受ける。
	var fields map[string]*string
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	// 途中でエラーになっても v に半端な値を残さないよう、parsed に集めてから入れる。
	var parsed valueJSON
	required := map[string]*string{
		"@tab":   &parsed.Tab,
		"@cat01": &parsed.Cat01,
		"@area":  &parsed.Area,
		"@time":  &parsed.Time,
		"$":      &parsed.Raw,
	}

	for key, target := range required {
		// キーが無いときも、値が null のときも nil になる。
		value := fields[key]
		if value == nil || *value == "" {
			return fmt.Errorf("e-Stat: VALUE key %q is missing or empty", key)
		}
		*target = *value
	}

	for key := range fields {
		if _, ok := required[key]; !ok && key != "@unit" {
			return fmt.Errorf("e-Stat: unknown VALUE key %q", key)
		}
	}

	if unit := fields["@unit"]; unit != nil {
		parsed.Unit = *unit
	}

	*v = parsed
	return nil
}

// getStatsDataResponse は getStatsData の応答全体を読む。使わないキーは読まずに捨てる。
type getStatsDataResponse struct {
	GetStatsData getStatsDataJSON `json:"GET_STATS_DATA"`
}

// getStatsDataJSON は GET_STATS_DATA の処理結果と統計データを読む。
// JSON の形をパッケージの外に出さず、内部で扱う。
type getStatsDataJSON struct {
	Result          resultJSON          `json:"RESULT"`
	StatisticalData statisticalDataJSON `json:"STATISTICAL_DATA"`
}

// resultJSON は RESULT の処理状態とメッセージを読む。
// Status は 0 が正常、1 が該当データなし（DATA_INF が無い）、100 が appId の誤りを表す。
type resultJSON struct {
	Status   int    `json:"STATUS"`
	ErrorMsg string `json:"ERROR_MSG"`
}

// statisticalDataJSON は STATISTICAL_DATA の続きの取得情報と値・注記を読む。
type statisticalDataJSON struct {
	ResultInf resultInfJSON `json:"RESULT_INF"`
	DataInf   dataInfJSON   `json:"DATA_INF"`
}

// resultInfJSON の NextKey は、続きが無いときはキーが無く 0 になる。
type resultInfJSON struct {
	NextKey int `json:"NEXT_KEY"`
}

// dataInfJSON は、値と記号の意味を照合するために DATA_INF の VALUE と NOTE を読む。
// VALUE と NOTE は、配列とオブジェクト1つのどちらでも受け付ける。
type dataInfJSON struct {
	Note  oneOrMany[noteJSON]  `json:"NOTE"`
	Value oneOrMany[valueJSON] `json:"VALUE"`
}
