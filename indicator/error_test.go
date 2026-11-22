package indicator

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestErrors(t *testing.T) {
	_, cause := strconv.Atoi("bad")
	de := &DefinitionError{Field: "Scale", Err: errors.Join(ErrInvalidDefinition, cause)}
	ie := &InputError{Index: 2, Area: "10201", Item: Item{TableID: "table", Code: "a"}, Year: 2024,
		Field: "Records[2].Value.Raw", Err: errors.Join(ErrInvalidRecord, cause)}
	for _, tt := range []struct {
		err   error
		class error
		field string
	}{
		{de, ErrInvalidDefinition, "Scale"},
		{ie, ErrInvalidRecord, "Records[2].Value.Raw"},
	} {
		var number *strconv.NumError
		if !errors.Is(tt.err, tt.class) || !errors.Is(tt.err, cause) || !errors.As(tt.err, &number) {
			t.Fatalf("lost error cause: %v", tt.err)
		}
		if !strings.Contains(tt.err.Error(), tt.field) || !strings.Contains(tt.err.Error(), cause.Error()) {
			t.Fatalf("missing location or cause: %v", tt.err)
		}
	}
	if de.Unwrap() != de.Err || ie.Unwrap() != ie.Err {
		t.Fatal("Unwrap does not return Err")
	}
	for _, text := range []string{"10201", "table", "a", "2024"} {
		if !strings.Contains(ie.Error(), text) {
			t.Errorf("InputError lacks %q: %v", text, ie)
		}
	}
}
