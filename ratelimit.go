package main

import (
	"io"
	"sync"
	"time"
)

// ramps bytes/sec up gradually after startup
type rampLimiter struct {
	start time.Time
	from  float64 // bytes/sec
	to    float64 // bytes/sec; 0 = unlimited (limiter is a no-op)
	ramp  time.Duration

	mu     sync.Mutex
	tokens float64
	last   time.Time
}

func newRampLimiter(fromMbps, toMbps float64, ramp time.Duration) *rampLimiter {
	now := time.Now()
	return &rampLimiter{
		start: now,
		from:  fromMbps * 1e6 / 8,
		to:    toMbps * 1e6 / 8,
		ramp:  ramp,
		last:  now,
	}
}

func (r *rampLimiter) currentRate() float64 {
	if r.to <= 0 {
		return 0
	}
	el := time.Since(r.start)
	if r.ramp <= 0 || el >= r.ramp {
		return r.to
	}
	frac := float64(el) / float64(r.ramp)
	return r.from + (r.to-r.from)*frac
}

func (r *rampLimiter) wait(n int) {
	for {
		curRate := r.currentRate()
		if curRate <= 0 {
			return
		}
		r.mu.Lock()
		now := time.Now()
		r.tokens += now.Sub(r.last).Seconds() * curRate
		r.last = now
		if r.tokens > curRate {
			r.tokens = curRate // at most ~1s of burst
		}
		if r.tokens >= float64(n) {
			r.tokens -= float64(n)
			r.mu.Unlock()
			return
		}
		need := float64(n) - r.tokens
		r.tokens = 0
		d := time.Duration(need / curRate * float64(time.Second))
		r.mu.Unlock()
		time.Sleep(d)
	}
}

type limitedWriter struct {
	w io.Writer
	l *rampLimiter
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	const chunk = 16 * 1024
	total := 0
	for len(p) > 0 {
		n := len(p)
		if n > chunk {
			n = chunk
		}
		lw.l.wait(n)
		written, err := lw.w.Write(p[:n])
		total += written
		if err != nil {
			return total, err
		}
		p = p[n:]
	}
	return total, nil
}
