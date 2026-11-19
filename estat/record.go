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
