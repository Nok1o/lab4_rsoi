package username

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid username")

const (
	minLength = 3
	maxLength = 80
)

func Validate(value string) error {
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return ErrInvalid
	}
	length := utf8.RuneCountInString(value)
	if length < minLength || length > maxLength {
		return ErrInvalid
	}
	return nil
}
