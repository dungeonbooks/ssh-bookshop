package main

import (
	"github.com/charmbracelet/wish/ratelimiter"
	"golang.org/x/time/rate"
)

// Connection limits: a blast door against a script opening sessions in a loop,
// not fairness between shoppers.
//
// Loose because wish keys on remote IP, and Railway's TCP proxy is layer 4 with
// no PROXY protocol, so on Railway every visitor collapses into one bucket. A
// limit tight enough to stop one abuser would shut the shop for everyone.
// Keying on the SSH public key instead would restore per-visitor buckets.
const (
	connRate  = rate.Limit(10) // sustained connections/sec per key (one key on Railway)
	connBurst = 30             // short spike allowed above that
	connCache = 1024           // distinct keys tracked
)

func connectionLimiter() ratelimiter.RateLimiter {
	return ratelimiter.NewRateLimiter(connRate, connBurst, connCache)
}
