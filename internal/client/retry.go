package client

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	defaultInitialRetryDelay = time.Second
	defaultMaximumRetryDelay = 15 * time.Second
	defaultStableSession     = 30 * time.Second
	defaultRetryJitter       = 0.20
)

type retrySettings struct {
	initial     time.Duration
	maximum     time.Duration
	stableAfter time.Duration
	jitter      float64
	random      func() float64
}

func defaultRetrySettings() retrySettings {
	return retrySettings{
		initial:     defaultInitialRetryDelay,
		maximum:     defaultMaximumRetryDelay,
		stableAfter: defaultStableSession,
		jitter:      defaultRetryJitter,
		random:      rand.Float64,
	}
}

type retryPolicy struct {
	settings retrySettings
	base     time.Duration
}

func newRetryPolicy(settings retrySettings) *retryPolicy {
	if settings.initial <= 0 {
		settings.initial = defaultInitialRetryDelay
	}
	if settings.maximum < settings.initial {
		settings.maximum = settings.initial
	}
	if settings.jitter < 0 {
		settings.jitter = 0
	}
	if settings.jitter > 1 {
		settings.jitter = 1
	}
	if settings.random == nil {
		settings.random = func() float64 { return 0.5 }
	}
	return &retryPolicy{settings: settings}
}

func (p *retryPolicy) nextDelay(connectedFor time.Duration) time.Duration {
	if p.base == 0 || (p.settings.stableAfter > 0 && connectedFor >= p.settings.stableAfter) {
		p.base = p.settings.initial
	} else {
		p.base = min(p.base*2, p.settings.maximum)
	}

	sample := p.settings.random()
	if sample < 0 {
		sample = 0
	}
	if sample > 1 {
		sample = 1
	}
	factor := 1 - p.settings.jitter + 2*p.settings.jitter*sample
	delay := time.Duration(float64(p.base) * factor)
	return min(delay, p.settings.maximum)
}

func reconnectMessage(delay time.Duration, err error) string {
	return fmt.Sprintf("Route unavailable; retrying in %s. %s", displayRetryDelay(delay), guidanceForClientError(err))
}

func displayRetryDelay(delay time.Duration) string {
	rounded := delay.Round(100 * time.Millisecond)
	if rounded <= 0 {
		rounded = 100 * time.Millisecond
	}
	return rounded.String()
}
