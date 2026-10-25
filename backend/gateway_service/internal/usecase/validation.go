package usecase

import (
	"fmt"

	usernamevalidation "libriary_system/shared/username"
)

type ValidationError struct {
	Fields map[string]string
}

func (err *ValidationError) Error() string {
	return "validation failed"
}

func validationError(fields map[string]string) error {
	if len(fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: fields}
}

func validateUsername(username string) error {
	if err := usernamevalidation.Validate(username); err != nil {
		return validationError(map[string]string{"username": fmt.Sprintf("%v", err)})
	}
	return nil
}
