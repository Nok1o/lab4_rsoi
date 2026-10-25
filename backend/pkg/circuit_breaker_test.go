package pkg

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCircuitBreakerOpensAndRecovers(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	breaker := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		MaxHalfOpen:      1,
		OpenTimeout:      time.Minute,
	})
	breaker.now = func() time.Time { return now }

	callError := errors.New("dependency unavailable")
	for range 2 {
		_, err := Do(breaker, func() (int, error) { return 0, callError })
		if !errors.Is(err, callError) {
			t.Fatalf("Do() error = %v, want %v", err, callError)
		}
	}
	if state := breaker.State(); state != StateOpen {
		t.Fatalf("State() = %s, want %s", state, StateOpen)
	}

	var calls int
	_, err := Do(breaker, func() (int, error) {
		calls++
		return 1, nil
	})
	if !errors.Is(err, CircuitBreakerOpenError) {
		t.Fatalf("Do() error = %v, want %v", err, CircuitBreakerOpenError)
	}
	if calls != 0 {
		t.Fatalf("callable invoked %d times while breaker was open", calls)
	}

	now = now.Add(time.Minute)
	result, err := Do(breaker, func() (int, error) {
		calls++
		return 42, nil
	})
	if err != nil {
		t.Fatalf("half-open Do() error = %v", err)
	}
	if result != 42 {
		t.Fatalf("half-open Do() result = %d, want 42", result)
	}
	if state := breaker.State(); state != StateClosed {
		t.Fatalf("State() = %s, want %s", state, StateClosed)
	}
}

func TestCircuitBreakerFailureInHalfOpenReopensIt(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	breaker := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		MaxHalfOpen:      1,
		OpenTimeout:      time.Minute,
	})
	breaker.now = func() time.Time { return now }

	callError := errors.New("dependency unavailable")
	_, _ = Do(breaker, func() (struct{}, error) { return struct{}{}, callError })
	now = now.Add(time.Minute)
	_, _ = Do(breaker, func() (struct{}, error) { return struct{}{}, callError })

	if state := breaker.State(); state != StateOpen {
		t.Fatalf("State() = %s, want %s", state, StateOpen)
	}
	if !breaker.openedAt.Equal(now) {
		t.Fatalf("openedAt = %s, want %s", breaker.openedAt, now)
	}
}

func TestCircuitBreakerLimitsConcurrentHalfOpenRequests(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	breaker := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		MaxHalfOpen:      1,
		OpenTimeout:      time.Minute,
	})
	breaker.now = func() time.Time { return now }

	_, _ = Do(breaker, func() (struct{}, error) {
		return struct{}{}, errors.New("dependency unavailable")
	})
	now = now.Add(time.Minute)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := Do(breaker, func() (struct{}, error) {
			close(started)
			<-release
			return struct{}{}, nil
		})
		done <- err
	}()
	<-started

	var secondCalled atomic.Bool
	_, err := Do(breaker, func() (struct{}, error) {
		secondCalled.Store(true)
		return struct{}{}, nil
	})
	if !errors.Is(err, CircuitBreakerTooManyRequestsError) {
		t.Fatalf("second Do() error = %v, want %v", err, CircuitBreakerTooManyRequestsError)
	}
	if secondCalled.Load() {
		t.Fatal("second callable was invoked after half-open limit was reached")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first half-open Do() error = %v", err)
	}
}

func TestCircuitBreakerIgnoresOldGenerationResult(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	breaker := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		MaxHalfOpen:      1,
		OpenTimeout:      time.Minute,
	})
	breaker.now = func() time.Time { return now }

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := Do(breaker, func() (struct{}, error) {
			close(started)
			<-release
			return struct{}{}, nil
		})
		done <- err
	}()
	<-started

	_, _ = Do(breaker, func() (struct{}, error) {
		return struct{}{}, errors.New("dependency unavailable")
	})
	if state := breaker.State(); state != StateOpen {
		t.Fatalf("State() = %s, want %s", state, StateOpen)
	}

	now = now.Add(time.Minute)
	if state := breaker.State(); state != StateHalfOpen {
		t.Fatalf("State() = %s, want %s", state, StateHalfOpen)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("old request error = %v", err)
	}
	if state := breaker.State(); state != StateHalfOpen {
		t.Fatalf("old result changed state to %s, want %s", state, StateHalfOpen)
	}
}

func TestCircuitBreakerExcludesCancelledRequest(t *testing.T) {
	breaker := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1})

	_, err := Do(breaker, func() (struct{}, error) {
		return struct{}{}, context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do() error = %v, want context.Canceled", err)
	}
	if state := breaker.State(); state != StateClosed {
		t.Fatalf("State() = %s, want %s", state, StateClosed)
	}
}

func TestCircuitBreakerUsesConfiguredFailureClassifier(t *testing.T) {
	applicationError := errors.New("not found")
	breaker := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		IsFailure: func(err error) bool {
			return err != nil && !errors.Is(err, applicationError)
		},
	})

	_, err := Do(breaker, func() (struct{}, error) {
		return struct{}{}, applicationError
	})
	if !errors.Is(err, applicationError) {
		t.Fatalf("Do() error = %v, want %v", err, applicationError)
	}
	if state := breaker.State(); state != StateClosed {
		t.Fatalf("State() = %s, want %s", state, StateClosed)
	}
}

func TestIsInfrastructureError(t *testing.T) {
	for _, check := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", want: false},
		{name: "client response", err: testStatusError(404), want: false},
		{name: "server response", err: testStatusError(503), want: true},
		{name: "network", err: errors.New("connection refused"), want: true},
	} {
		t.Run(check.name, func(t *testing.T) {
			if got := IsInfrastructureError(check.err); got != check.want {
				t.Fatalf("IsInfrastructureError() = %t, want %t", got, check.want)
			}
		})
	}
}

type testStatusError int

func (err testStatusError) Error() string {
	return "HTTP response error"
}

func (err testStatusError) StatusCode() int {
	return int(err)
}

func TestCircuitBreakerFailureTimestampWindow(t *testing.T) {
	for _, check := range []struct {
		name  string
		times []time.Duration
		want  State
	}{
		{"below count threshold", []time.Duration{0, time.Second}, StateClosed},
		{"opens on third failure", []time.Duration{0, 10 * time.Second, 20 * time.Second}, StateOpen},
		{"exactly one minute does not open", []time.Duration{0, 20 * time.Second, time.Minute}, StateClosed},
		{"one nanosecond below minute opens", []time.Duration{0, 20 * time.Second, time.Minute - time.Nanosecond}, StateOpen},
		{"old failures do not open", []time.Duration{0, time.Minute, 2 * time.Minute}, StateClosed},
		{"overwritten failures leave recent window", []time.Duration{0, 30 * time.Second, time.Minute, 90 * time.Second, 119 * time.Second}, StateOpen},
	} {
		t.Run(check.name, func(t *testing.T) {
			start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
			now := start
			breaker := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3})
			breaker.now = func() time.Time { return now }
			storage := &breaker.results[0]
			callError := errors.New("unavailable")
			for _, at := range check.times {
				now = start.Add(at)
				_, err := Do(breaker, func() (struct{}, error) { return struct{}{}, callError })
				if !errors.Is(err, callError) {
					t.Fatalf("Do() error = %v, want %v", err, callError)
				}
			}
			if state := breaker.State(); state != check.want {
				t.Fatalf("State() = %s, want %s", state, check.want)
			}
			if len(breaker.results) != 3 || &breaker.results[0] != storage {
				t.Fatal("failure ring changed capacity or backing array")
			}
			if breaker.resultsCount != min(len(check.times), 3) {
				t.Fatalf("stored failures = %d", breaker.resultsCount)
			}
			first := max(0, len(check.times)-3)
			head := 0
			if breaker.resultsCount == 3 {
				head = breaker.resultsHead
			}
			for i, at := range check.times[first:] {
				got := breaker.results[(head+i)%len(breaker.results)]
				if want := start.Add(at); !got.Equal(want) {
					t.Fatalf("failure %d timestamp = %s, want %s", i, got, want)
				}
			}
		})
	}
}

func TestCircuitBreakerSuccessesAndExcludedErrorsDoNotChangeRing(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	breaker := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2})
	breaker.now = func() time.Time { return now }
	callError := errors.New("unavailable")
	_, _ = Do(breaker, func() (struct{}, error) { return struct{}{}, callError })
	stored := append([]time.Time(nil), breaker.results...)
	head, count := breaker.resultsHead, breaker.resultsCount
	for _, outcome := range []error{nil, testStatusError(404), context.Canceled} {
		now = now.Add(time.Second)
		_, err := Do(breaker, func() (struct{}, error) { return struct{}{}, outcome })
		if !errors.Is(err, outcome) {
			t.Fatalf("Do() error = %v, want %v", err, outcome)
		}
		if breaker.resultsHead != head || breaker.resultsCount != count {
			t.Fatal("non-failure changed ring position or count")
		}
		for i, timestamp := range stored {
			if !breaker.results[i].Equal(timestamp) {
				t.Fatal("non-failure changed a stored failure timestamp")
			}
		}
	}
	now = now.Add(time.Second)
	_, _ = Do(breaker, func() (struct{}, error) { return struct{}{}, callError })
	if state := breaker.State(); state != StateOpen {
		t.Fatalf("successes reset failure history: State() = %s", state)
	}
}

func TestCircuitBreakerRecoveryResetsFailureRing(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	breaker := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, OpenTimeout: time.Second})
	breaker.now = func() time.Time { return now }
	callError := errors.New("unavailable")
	for range 2 {
		_, _ = Do(breaker, func() (struct{}, error) { return struct{}{}, callError })
	}
	storage := &breaker.results[0]
	now = now.Add(time.Second)
	_, err := Do(breaker, func() (struct{}, error) { return struct{}{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if breaker.State() != StateClosed || breaker.resultsCount != 0 || breaker.resultsHead != 0 {
		t.Fatal("recovery did not reset failure ring")
	}
	if &breaker.results[0] != storage {
		t.Fatal("recovery reallocated ring")
	}
	for _, timestamp := range breaker.results {
		if !timestamp.IsZero() {
			t.Fatal("recovery left old timestamps")
		}
	}
	_, _ = Do(breaker, func() (struct{}, error) { return struct{}{}, callError })
	if breaker.State() != StateClosed || breaker.resultsCount != 1 {
		t.Fatal("old failures reopened recovered breaker")
	}
}

func TestCircuitBreakerRecordsConcurrentFailuresOnly(t *testing.T) {
	const requests = 100
	breaker := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: requests})
	var group sync.WaitGroup
	for i := range requests {
		group.Go(func() {
			_, _ = Do(breaker, func() (struct{}, error) {
				if i%2 == 0 {
					return struct{}{}, errors.New("unavailable")
				}
				return struct{}{}, nil
			})
		})
	}
	group.Wait()
	if breaker.State() != StateClosed || breaker.resultsCount != requests/2 {
		t.Fatalf("state=%s stored failures=%d, want CLOSED and %d", breaker.State(), breaker.resultsCount, requests/2)
	}
}

func TestCircuitBreakerFailureThresholdDefaults(t *testing.T) {
	for _, threshold := range []int{0, -1, 1, 128} {
		breaker := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: threshold})
		want := threshold
		if want <= 0 {
			want = 3
		}
		if breaker.config.FailureThreshold != want || len(breaker.results) != want {
			t.Fatalf("threshold %d: configured=%d ring capacity=%d, want %d", threshold, breaker.config.FailureThreshold, len(breaker.results), want)
		}
	}
}
