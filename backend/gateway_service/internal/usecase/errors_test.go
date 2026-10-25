package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/pkg"
)

type statusError int

func (e statusError) Error() string   { return fmt.Sprintf("HTTP %d", e) }
func (e statusError) StatusCode() int { return int(e) }

func TestIsRetryable(t *testing.T) {
	for _, test := range []struct {
		err  error
		want bool
	}{
		{nil, false}, {domain.ErrBookNotFound, false}, {domain.ErrRatingNotFound, false},
		{&DependencyError{Cause: statusError(500)}, true},
		{&DependencyError{Cause: statusError(503)}, true},
		{&DependencyError{Cause: statusError(400)}, false},
		{&DependencyError{Cause: statusError(409)}, false},
		{&DependencyError{Cause: statusError(429)}, true},
		{&DependencyError{Cause: context.DeadlineExceeded}, true},
		{&DependencyError{Cause: context.Canceled}, false},
		{&DependencyError{Cause: errors.New("connection refused")}, true},
		{&DependencyError{Cause: pkg.CircuitBreakerOpenError}, true},
	} {
		if got := IsRetryable(test.err); got != test.want {
			t.Errorf("IsRetryable(%v) = %t", test.err, got)
		}
	}
}
