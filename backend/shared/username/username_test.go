package username

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		username string
		valid    bool
	}{
		{name: "minimum length", username: "abc", valid: true},
		{name: "maximum length", username: strings.Repeat("a", 80), valid: true},
		{name: "unicode", username: "Иван", valid: true},
		{name: "unicode maximum length", username: strings.Repeat("я", 80), valid: true},
		{name: "short", username: "ab"},
		{name: "empty", username: ""},
		{name: "only spaces", username: "   "},
		{name: "leading space", username: " abc"},
		{name: "trailing space", username: "abc "},
		{name: "too long", username: strings.Repeat("a", 81)},
		{name: "invalid UTF-8", username: "ab\xff"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.username)
			if test.valid && err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", test.username, err)
			}
			if !test.valid && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate(%q) = %v, want ErrInvalid", test.username, err)
			}
		})
	}
}
