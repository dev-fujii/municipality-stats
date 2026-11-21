package estat

import (
	"fmt"
	"strings"
)

// noteTexts は、扱う記号と、NOTE に載る意味の文の対応。
// 「-」が「該当数値なし」を表す統計もあるので、文まで照らして、別の種類の統計表を渡されたときに止める。
var noteTexts = map[string]string{
	"***": "調査又は集計していないもの",
	"-":   "データが得られないもの",
	"X":   "数値が秘匿されているもの",
}

// checkNotes は、NOTE の記号と意味の文を確かめ、載っていた記号の集合を返す。
// 返した集合は、VALUE の記号が NOTE に載っているかを確かめるのに使う。
func checkNotes(notes []noteJSON) (map[string]bool, error) {
	seen := make(map[string]bool, len(notes))
	for _, n := range notes {
		want, ok := noteTexts[n.Char]
		if !ok {
			return nil, fmt.Errorf("e-Stat: unknown NOTE symbol %q", n.Char)
		}
		if got := strings.Join(strings.Fields(n.Text), " "); got != want {
			return nil, fmt.Errorf("e-Stat: NOTE text for %q is %q, want %q", n.Char, n.Text, want)
		}
		seen[n.Char] = true
	}
	return seen, nil
}

// Record は、統計表の値の1件を、統計表・項目・地域・年で特定できる形で持つ。
type Record struct {
	TableID string // 統計表 ID
	Code    string // 項目コード（@cat01）
	Area    string // 地域コード（@area。5桁）
	Year    int    // @time の先頭4桁
	Unit    string // @unit。無いときは空
	Value   Value
}

// recordKey は、同じ Record が2つあるかを確かめる鍵。統計表は builder ごとに1つなので持たない。
type recordKey struct {
	code string
	area string
	year int
}

// recordBuilder は、1つの統計表の応答を順に Record に変える。
// 統計表は続きの取得で複数の応答に分かれるので、@tab と重複の検査のために応答をまたいで状態を持つ。
type recordBuilder struct {
	tableID string
	tab     string
	seen    map[recordKey]bool
	records []Record
}

func newRecordBuilder(tableID string) *recordBuilder {
	return &recordBuilder{tableID: tableID, seen: make(map[recordKey]bool)}
}

// add は、1つの応答の DATA_INF を確かめて足す。記号は同じ応答の NOTE だけで照らす。
func (b *recordBuilder) add(d dataInfJSON) error {
	notes, err := checkNotes(d.Note)
	if err != nil {
		return err
	}

	// 途中でエラーになっても、次の応答の検証に影響を残さないよう一時変数に集める。
	tab := b.tab
	seen := make(map[recordKey]bool, len(d.Value))
	records := make([]Record, 0, len(d.Value))
	for i, v := range d.Value {
		if tab == "" {
			tab = v.Tab
		} else if v.Tab != tab {
			return fmt.Errorf("e-Stat: VALUE[%d]: @tab %q differs from %q", i, v.Tab, tab)
		}

		year, err := parseYear(v.Time)
		if err != nil {
			return fmt.Errorf("e-Stat: VALUE[%d]: @time %q: %w", i, v.Time, err)
		}
		value, err := parseValue(v.Raw)
		if err != nil {
			return fmt.Errorf("e-Stat: VALUE[%d]: value %q: %w", i, v.Raw, err)
		}
		if value.Status != StatusOK && !notes[v.Raw] {
			return fmt.Errorf("e-Stat: VALUE[%d]: symbol %q is not in NOTE", i, v.Raw)
		}

		key := recordKey{code: v.Cat01, area: v.Area, year: year}
		if b.seen[key] || seen[key] {
			return fmt.Errorf("e-Stat: VALUE[%d]: duplicate record for code %q, area %q, @time %q", i, v.Cat01, v.Area, v.Time)
		}
		seen[key] = true
		records = append(records, Record{
			TableID: b.tableID,
			Code:    v.Cat01,
			Area:    v.Area,
			Year:    year,
			Unit:    v.Unit,
			Value:   value,
		})
	}

	b.tab = tab
	for key := range seen {
		b.seen[key] = true
	}
	b.records = append(b.records, records...)
	return nil
}

func (b *recordBuilder) result() []Record {
	return b.records
}
