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
