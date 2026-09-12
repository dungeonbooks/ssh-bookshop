package main

import (
	"net/http"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

// API limits, in the same three-layer shape as the SSH connection limiter:
// per address for fairness, shop-wide for survival. There is no per-key layer
// because HTTP callers have no key.
//
// Checkout has its own pair of buckets on top, tighter, because each call
// creates an order and a payment link on Square. The sweeper cleans those up
// after a day, so the cost of a flood is bounded, but a bounded mess is still
// a mess.
const (
	apiAddrRate  = 5
	apiAddrBurst = 20
	apiShopRate  = 50
	apiShopBurst = 100

	checkoutAddrRate  = rate.Limit(0.2) // one every five seconds, sustained
	checkoutAddrBurst = 3
	checkoutShopRate  = 2
	checkoutShopBurst = 10

	apiAddrCache = 1024
)

// bucketCache is one LRU of per-key limiters, locked because get-then-add is
// not atomic.
type bucketCache struct {
	mu sync.Mutex
	c  *lru.Cache[string, *rate.Limiter]
	b  bucket
}

func newBucketCache(size int, b bucket) *bucketCache {
	return &bucketCache{c: newLimiterCache(size), b: b}
}

func (bc *bucketCache) get(key string) *rate.Limiter {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if lim, ok := bc.c.Get(key); ok {
		return lim
	}
	lim := rate.NewLimiter(bc.b.r, bc.b.burst)
	bc.c.Add(key, lim)
	return lim
}

type apiLimiter struct {
	addrs        *bucketCache
	shop         *rate.Limiter
	checkoutAddr *bucketCache
	checkoutShop *rate.Limiter
}

func newAPILimiter() *apiLimiter {
	return &apiLimiter{
		addrs:        newBucketCache(apiAddrCache, bucket{apiAddrRate, apiAddrBurst}),
		shop:         rate.NewLimiter(apiShopRate, apiShopBurst),
		checkoutAddr: newBucketCache(apiAddrCache, bucket{checkoutAddrRate, checkoutAddrBurst}),
		checkoutShop: rate.NewLimiter(checkoutShopRate, checkoutShopBurst),
	}
}

// allow takes one token from the address bucket and one from the shop bucket,
// handing the first back if the second refuses, as the SSH limiter does.
func allowPair(addr, shop *rate.Limiter) bool {
	now := time.Now()
	res, ok := reserve(addr, now)
	if !ok {
		return false
	}
	if _, ok := reserve(shop, now); !ok {
		res.CancelAt(now)
		return false
	}
	return true
}

func (l *apiLimiter) allow(key string) bool {
	return allowPair(l.addrs.get(key), l.shop)
}

func (l *apiLimiter) allowCheckout(key string) bool {
	return allowPair(l.checkoutAddr.get(key), l.checkoutShop)
}

func (l *apiLimiter) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientKey(r)) {
			w.Header().Set("Retry-After", "1")
			writeErr(w, apiErr(http.StatusTooManyRequests, "too many requests, slow down"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
