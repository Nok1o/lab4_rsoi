package pkg

import (
	"context"
	"errors"
	"sync"
	"time"
)

type State string

const (
	StateOpen     State = "OPEN"
	StateClosed   State = "CLOSED"
	StateHalfOpen State = "HALF_OPEN"

	requestWindow = time.Minute
)

var (
	CircuitBreakerOpenError            = errors.New("circuit breaker is open")
	CircuitBreakerTooManyRequestsError = errors.New(
		"circuit breaker has reached the half-open request limit",
	)
)

type requestToken struct {
	generation uint64
}

type requestOutcome uint8

const (
	outcomeSuccess requestOutcome = iota
	outcomeFailure
	outcomeExcluded
)

type CircuitBreaker struct {
	mu sync.Mutex

	state State

	results      []time.Time
	resultsHead  int
	resultsCount int

	halfOpenRequests  int
	halfOpenSuccesses int

	generation uint64
	openedAt   time.Time

	config CircuitBreakerConfig
	now    func() time.Time
}

func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	config = config.withDefaults()
	return &CircuitBreaker{
		state:      StateClosed,
		results:    make([]time.Time, config.FailureThreshold),
		generation: 1,
		config:     config,
		now:        time.Now,
	}
}

// Do executes callable when breaker permits a request and records its result.
// The callable is deliberately executed without holding the breaker mutex.
func Do[T any](breaker *CircuitBreaker, callable func() (T, error)) (result T, err error) {
	token, err := breaker.beforeRequest()
	if err != nil {
		return result, err
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			breaker.afterRequest(token, outcomeFailure)
			panic(recovered)
		}
	}()

	result, err = callable()
	breaker.afterRequest(token, breaker.classify(err))
	return result, err
}

// State returns the current breaker state. Reading the state also performs the
// time-based OPEN -> HALF_OPEN transition.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.updateState(cb.now())
	return cb.state
}

func (cb *CircuitBreaker) beforeRequest() (requestToken, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.updateState(cb.now())

	switch cb.state {
	case StateClosed:
		return requestToken{generation: cb.generation}, nil
	case StateOpen:
		return requestToken{}, CircuitBreakerOpenError
	case StateHalfOpen:
		if cb.halfOpenRequests >= cb.config.MaxHalfOpen {
			return requestToken{}, CircuitBreakerTooManyRequestsError
		}
		cb.halfOpenRequests++
		return requestToken{generation: cb.generation}, nil
	default:
		panic("unknown circuit breaker state")
	}
}

func (cb *CircuitBreaker) afterRequest(token requestToken, outcome requestOutcome) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// The breaker may have changed state while the request was running. Such a
	// result belongs to an old generation and must not affect current counters.
	if token.generation != cb.generation {
		return
	}

	switch cb.state {
	case StateClosed:
		cb.completeClosed(outcome)
	case StateHalfOpen:
		cb.completeHalfOpen(outcome)
	case StateOpen:
		return
	default:
		panic("unknown circuit breaker state")
	}
}

func (cb *CircuitBreaker) completeClosed(outcome requestOutcome) {
	switch outcome {
	case outcomeFailure:
		now := cb.now()
		if cb.recordFailure(now) {
			cb.toOpen(now)
		}
	case outcomeSuccess, outcomeExcluded:
		return
	default:
		panic("unknown circuit breaker request outcome")
	}
}

func (cb *CircuitBreaker) completeHalfOpen(outcome requestOutcome) {
	switch outcome {
	case outcomeSuccess:
		cb.halfOpenSuccesses++
		if cb.halfOpenSuccesses >= cb.config.MaxHalfOpen {
			cb.toClosed()
		}
	case outcomeFailure:
		now := cb.now()
		cb.recordFailure(now)
		cb.toOpen(now)
	case outcomeExcluded:
		// The excluded request did not test service health, so release its slot
		// for another probe in this half-open generation.
		cb.halfOpenRequests--
	default:
		panic("unknown circuit breaker request outcome")
	}
}

func (cb *CircuitBreaker) updateState(now time.Time) {
	if cb.state == StateOpen && now.Sub(cb.openedAt) >= cb.config.OpenTimeout {
		cb.toHalfOpen()
	}
}

// recordFailure overwrites the oldest failure in O(1), with no allocations.
// Compare after writing so N failures, not N+1, trigger the transition.
// Call under mu.
func (cb *CircuitBreaker) recordFailure(now time.Time) bool {
	cb.results[cb.resultsHead] = now
	cb.resultsHead = (cb.resultsHead + 1) % len(cb.results)
	cb.resultsCount = min(cb.resultsCount+1, len(cb.results))
	return cb.resultsCount == len(cb.results) &&
		now.Sub(cb.results[cb.resultsHead]) < requestWindow
}

func (cb *CircuitBreaker) toOpen(now time.Time) {
	cb.state = StateOpen
	cb.openedAt = now
	cb.halfOpenRequests = 0
	cb.halfOpenSuccesses = 0
	cb.generation++
}

func (cb *CircuitBreaker) toHalfOpen() {
	cb.state = StateHalfOpen
	cb.halfOpenRequests = 0
	cb.halfOpenSuccesses = 0
	cb.generation++
}

func (cb *CircuitBreaker) toClosed() {
	cb.state = StateClosed
	cb.openedAt = time.Time{}
	// A successful recovery starts a fresh observation window.
	clear(cb.results)
	cb.resultsHead = 0
	cb.resultsCount = 0
	cb.halfOpenRequests = 0
	cb.halfOpenSuccesses = 0
	cb.generation++
}

func (cb *CircuitBreaker) classify(err error) requestOutcome {
	if cb.config.IsExcluded(err) {
		return outcomeExcluded
	}
	if cb.config.IsFailure(err) {
		return outcomeFailure
	}
	return outcomeSuccess
}

func defaultIsExcluded(err error) bool {
	return errors.Is(err, context.Canceled)
}

type statusCodeError interface {
	StatusCode() int
}

// IsInfrastructureError classifies errors that indicate an unavailable or
// unhealthy dependency. An HTTP 4xx response proves that the service answered
// and therefore must not trip the breaker.
func IsInfrastructureError(err error) bool {
	if err == nil {
		return false
	}

	var responseErr statusCodeError
	if errors.As(err, &responseErr) {
		return responseErr.StatusCode() >= 500
	}

	// Network errors, timeouts and malformed responses have no HTTP status and
	// are treated as dependency failures.
	return true
}
