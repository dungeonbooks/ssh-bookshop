package main

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

// openLimiter is the real limiter with the named layers left wide open, so a
// test can watch one set of buckets without the others interfering.
func openLimiter(t *testing.T, open ...string) *keyedLimiter {
	t.Helper()
	wide := bucket{rate.Inf, 0}
	l := &keyedLimiter{
		keys:    newLimiterCache(keyCache),
		addrs:   newLimiterCache(addrCache),
		keyLim:  bucket{keyRate, keyBurst},
		addrLim: bucket{addrRate, addrBurst},
		shop:    rate.NewLimiter(shopRate, shopBurst),
	}
	for _, layer := range open {
		switch layer {
		case "shop":
			l.shop = rate.NewLimiter(rate.Inf, 0)
		case "addr":
			l.addrLim = wide
		case "key":
			l.keyLim = wide
		default:
			t.Fatalf("unknown layer %q", layer)
		}
	}
	return l
}

const oneAddr = "203.0.113.7"

// The whole point of keying on the fingerprint: one visitor hammering the shop
// must not spend anyone else's allowance.
func TestOneKeyDoesNotExhaustAnother(t *testing.T) {
	l := openLimiter(t, "shop", "addr")

	for i := 0; i < keyBurst; i++ {
		if !l.allow("noisy", oneAddr) {
			t.Fatalf("noisy was refused on connection %d, inside its own burst", i+1)
		}
	}
	if l.allow("noisy", oneAddr) {
		t.Error("noisy kept going past its burst")
	}
	if !l.allow("quiet", oneAddr) {
		t.Error("quiet was refused because noisy had been busy")
	}
}

// The gap this layer exists to close. A fresh keypair costs an abuser nothing
// and arrives with a full bucket, so per-key limiting never refuses them. Before
// the address bucket, the shop-wide ceiling was the only thing left, which meant
// one rotating client could drain the entire shop's allowance and lock out
// everybody else.
func TestKeyRotationCannotDrainTheShop(t *testing.T) {
	l := openLimiter(t)
	const abuser, victim = "198.51.100.9", "203.0.113.7"

	// Every connection presents a key never seen before, so no per-key bucket
	// can refuse; only the address bucket stands in the way.
	allowed := 0
	for i := 0; i < shopBurst*3; i++ {
		if l.allow(fmt.Sprintf("fresh-key-%d", i), abuser) {
			allowed++
		}
	}
	if allowed > addrBurst+1 { // +1 for a token the limiter may accrue mid-loop
		t.Errorf("%d connections allowed from %d fresh keys on one address, want about the %d burst",
			allowed, shopBurst*3, addrBurst)
	}

	// The real assertion: after all that, the shop still has room for someone
	// else. This is what failed before the address layer existed.
	if !l.allow("honest-shopper", victim) {
		t.Error("an honest visitor was refused after one address rotated keys at the shop")
	}
}

// The ceiling still has to hold when the abuse is genuinely distributed, since
// the address layer cannot help once every connection comes from somewhere new.
func TestShopCeilingBoundsDistributedRotation(t *testing.T) {
	l := openLimiter(t)

	allowed := 0
	for i := 0; i < shopBurst*3; i++ {
		if l.allow(fmt.Sprintf("key-%d", i), fmt.Sprintf("198.51.100.%d", i%256)) {
			allowed++
		}
	}
	if allowed > shopBurst+1 {
		t.Errorf("%d connections allowed from %d fresh keys and addresses, want about the %d ceiling",
			allowed, shopBurst*3, shopBurst)
	}
	if allowed == 0 {
		t.Error("the ceiling refused everything")
	}
}

// A refusal must not charge the layers that already said yes. Otherwise a flood
// drains the personal allowance of every shopper it turns away, and they stay
// locked out after it stops.
func TestRefusalRefundsTheNarrowerBuckets(t *testing.T) {
	l := openLimiter(t, "shop")

	// Exhaust one address with throwaway keys, so the address bucket is what
	// refuses from here on.
	for i := 0; l.allow(fmt.Sprintf("burner-%d", i), oneAddr); i++ {
		if i > addrBurst*4 {
			t.Fatal("the address bucket never refused")
		}
	}

	// A shopper on that address is now refused, but must not have been billed
	// for it. Once they move to an address with room, their own burst should be
	// untouched.
	const shopper = "honest"
	if l.allow(shopper, oneAddr) {
		t.Fatal("the address bucket should still be empty")
	}
	for i := 0; i < keyBurst; i++ {
		if !l.allow(shopper, fmt.Sprintf("198.51.100.%d", i)) {
			t.Fatalf("shopper refused on connection %d: their key bucket was charged for an address refusal", i+1)
		}
	}
}

// The fallback key must not carry the ephemeral port: a per-connection key is
// no limit at all, and a thousand of them evict every real entry from the LRU
// on the way past.
func TestAddrKeyDropsThePort(t *testing.T) {
	ip := net.ParseIP("203.0.113.7")
	first := addrKey(&net.TCPAddr{IP: ip, Port: 54321})
	second := addrKey(&net.TCPAddr{IP: ip, Port: 54322})
	if first != second {
		t.Errorf("two connections from one IP keyed differently: %q vs %q", first, second)
	}
	if strings.Contains(first, "54321") {
		t.Errorf("addrKey = %q, want no port in it", first)
	}
	if first != "203.0.113.7" {
		t.Errorf("addrKey = %q, want the bare IP", first)
	}
}

// A host gets a whole /64 to itself, so anything finer than this bounds nothing:
// the occupant picks a new address per connection for free.
func TestAddrKeyCollapsesIPv6ToItsPrefix(t *testing.T) {
	prefix := addrKey(&net.TCPAddr{IP: net.ParseIP("2001:db8:dead:beef::1"), Port: 1})
	sibling := addrKey(&net.TCPAddr{IP: net.ParseIP("2001:db8:dead:beef:ffff:ffff:ffff:ffff"), Port: 2})
	if prefix != sibling {
		t.Errorf("two addresses in one /64 keyed differently: %q vs %q", prefix, sibling)
	}

	elsewhere := addrKey(&net.TCPAddr{IP: net.ParseIP("2001:db8:dead:bee0::1"), Port: 3})
	if prefix == elsewhere {
		t.Errorf("a different /64 shares the key %q", prefix)
	}
}
