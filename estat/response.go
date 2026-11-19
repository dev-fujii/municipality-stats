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
