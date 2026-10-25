package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

type DependencyError struct {
	Service string
	Cause   error
}

func (err *DependencyError) Error() string {
	return fmt.Sprintf("%s request failed: %v", err.Service, err.Cause)
}

func (err *DependencyError) Unwrap() error {
	return err.Cause
}

// IsRetryable distinguishes transport/server failures from permanent domain
// errors. StatusCode is implemented by the HTTP clients' error types.
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var status interface{ StatusCode() int }
	if errors.As(err, &status) {
		return status.StatusCode() >= 500 || status.StatusCode() == http.StatusRequestTimeout || status.StatusCode() == http.StatusTooManyRequests
	}
	var dependency *DependencyError
	return errors.As(err, &dependency)
}
