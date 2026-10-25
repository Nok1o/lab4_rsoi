package pkg

import "time"

const (
	defaultFailureThreshold = 3
	defaultMaxHalfOpen      = 1
	defaultOpenTimeout      = 10 * time.Second
)

// CircuitBreakerConfig configures transitions between the CLOSED, OPEN and
// HALF_OPEN states.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures within less than one minute
	// required to open the breaker, and the capacity of its timestamp ring.
	FailureThreshold int
	MaxHalfOpen      int
	OpenTimeout      time.Duration

	// IsFailure reports whether err means that the protected service is
	// unavailable. By default every non-nil, non-excluded error is a failure.
	IsFailure func(error) bool

	// IsExcluded reports errors that must not affect breaker statistics. A
	// cancelled caller context is excluded by default.
	IsExcluded func(error) bool
}

func (config CircuitBreakerConfig) withDefaults() CircuitBreakerConfig {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = defaultFailureThreshold
	}
	if config.MaxHalfOpen <= 0 {
		config.MaxHalfOpen = defaultMaxHalfOpen
	}
	if config.OpenTimeout <= 0 {
		config.OpenTimeout = defaultOpenTimeout
	}
	if config.IsFailure == nil {
		config.IsFailure = IsInfrastructureError
	}
	if config.IsExcluded == nil {
		config.IsExcluded = defaultIsExcluded
	}
	return config
}
