package main

import (
	"testing"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

// testLimiter is the real limiter with the shop-wide bucket opened right up, so
// a test can watch the per-key buckets without the ceiling interfering.
func testLimiter(t *testing.T) *keyedLimiter {
	t.Helper()
	cache, err := lru.New[string, *rate.Limiter](connCache)
	if err != nil {
		t.Fatal(err)
	}
	return &keyedLimiter{cache: cache, shop: rate.NewLimiter(rate.Inf, 0)}
}

// The whole point of keying on the fingerprint: one visitor hammering the shop
// must not spend anyone else's allowance. Under the old IP key on Railway it
// did, because everyone shared a bucket.
func TestOneKeyDoesNotExhaustAnother(t *testing.T) {
	l := testLimiter(t)

	for i := 0; i < keyBurst; i++ {
		if !l.allow("noisy") {
			t.Fatalf("noisy was refused on connection %d, inside its own burst", i+1)
		}
	}
	if l.allow("noisy") {
		t.Error("noisy kept going past its burst")
	}
	if !l.allow("quiet") {
		t.Error("quiet was refused because noisy had been busy")
	}
}

// Per-key buckets are free to mint: a fresh keypair costs an abuser nothing, so
// the shop-wide ceiling is the only thing standing between key rotation and an
// unbounded connection rate.
func TestShopCeilingBoundsKeyRotation(t *testing.T) {
	cache, err := lru.New[string, *rate.Limiter](connCache)
	if err != nil {
		t.Fatal(err)
	}
	l := &keyedLimiter{cache: cache, shop: rate.NewLimiter(shopRate, shopBurst)}

	// Every connection uses a key never seen before, so no per-key bucket ever
	// rejects and only the ceiling can.
	allowed := 0
	for i := 0; i < shopBurst*3; i++ {
		if l.allow(string(rune('a'+i%26)) + string(rune('0'+i/26))) {
			allowed++
		}
	}
	if allowed > shopBurst+1 { // +1 for a token the limiter may accrue mid-loop
		t.Errorf("%d connections allowed from %d fresh keys, want about the %d burst",
			allowed, shopBurst*3, shopBurst)
	}
	if allowed == 0 {
		t.Error("the ceiling refused everything")
	}
}
