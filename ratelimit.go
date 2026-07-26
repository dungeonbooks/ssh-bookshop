package main

import (
	"net"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"charm.land/wish/v2/ratelimiter"
	"github.com/charmbracelet/ssh"
	lru "github.com/hashicorp/golang-lru/v2"
	gossh "golang.org/x/crypto/ssh"
)

// Connection limits: a blast door against a script opening sessions in a loop,
// not fairness between shoppers.
//
// Three buckets, and a session needs room in all of them.
//
// Per public key is fairness. It gives every visitor their own allowance so one
// busy client cannot spend anyone else's. On its own it stops nobody, because a
// fresh keypair is free and arrives with a full bucket.
//
// Per source address is what actually bounds key rotation. Minting keys costs an
// abuser nothing; obtaining addresses costs them something. Without this layer
// a single rotating client can drain the shop-wide bucket and every honest
// visitor is refused on a shop with capacity to spare. Kept deliberately loose,
// because CGNAT puts unrelated shoppers behind one address and this is a bound,
// not a quota.
//
// Shop-wide is survival, the ceiling nothing gets past.
//
// The address layer is only meaningful because the shop takes connections
// directly. Behind Railway's layer 4 proxy, which carried no PROXY protocol,
// every visitor arrived from the same address and an address bucket would have
// been a second shop-wide bucket wearing a disguise.
const (
	keyRate  = 1 // sustained connections/sec for one public key
	keyBurst = 5 // a few quick reconnects are normal

	// Loose on purpose: this has to sit above what a shared NAT of real
	// shoppers produces, while still being far below the shop-wide ceiling so
	// that one address cannot reach it.
	addrRate  = 3
	addrBurst = 12

	shopRate  = 10
	shopBurst = 30

	keyCache  = 1024 // distinct fingerprints tracked
	addrCache = 1024 // distinct source addresses tracked
)

// bucket is the shape of the per-visitor limiters, held as data so a test can
// open one layer right up and watch another in isolation.
type bucket struct {
	r     rate.Limit
	burst int
}

// keyedLimiter allows a session when its key's bucket, its address's bucket and
// the shop-wide bucket all have room.
type keyedLimiter struct {
	mu      sync.Mutex
	keys    *lru.Cache[string, *rate.Limiter]
	addrs   *lru.Cache[string, *rate.Limiter]
	keyLim  bucket
	addrLim bucket
	shop    *rate.Limiter
}

func connectionLimiter() ratelimiter.RateLimiter {
	return &keyedLimiter{
		keys:    newLimiterCache(keyCache),
		addrs:   newLimiterCache(addrCache),
		keyLim:  bucket{keyRate, keyBurst},
		addrLim: bucket{addrRate, addrBurst},
		shop:    rate.NewLimiter(shopRate, shopBurst),
	}
}

func newLimiterCache(size int) *lru.Cache[string, *rate.Limiter] {
	cache, err := lru.New[string, *rate.Limiter](size)
	if err != nil {
		// Only reachable by editing a cache size to a non-positive value, which
		// is a programming error rather than a runtime condition. Failing here
		// says so; swallowing it returns a nil cache that panics on the first
		// connection instead, a long way from the cause.
		panic("connection limiter cache: " + err.Error())
	}
	return cache
}

func (l *keyedLimiter) Allow(s ssh.Session) error {
	if !l.allow(sessionKey(s), addrKey(s.RemoteAddr())) {
		return ratelimiter.ErrRateLimitExceeded
	}
	return nil
}

// allow is the decision without the session around it, so the buckets can be
// tested without standing up an SSH server.
//
// Tokens are reserved rather than spent, and a later refusal hands back what the
// earlier layers took. Otherwise a visitor refused by the ceiling would still be
// charged for it, and a flood would quietly drain the personal allowance of
// every shopper it turned away — punishing them twice for someone else's abuse.
func (l *keyedLimiter) allow(key, addr string) bool {
	now := time.Now()

	// Narrowest bucket first, so a noisy client is refused on its own allowance
	// before it can disturb anything shared.
	keyRes, ok := reserve(l.limiterFor(l.keys, key, l.keyLim), now)
	if !ok {
		return false
	}
	addrRes, ok := reserve(l.limiterFor(l.addrs, addr, l.addrLim), now)
	if !ok {
		keyRes.CancelAt(now)
		return false
	}
	if _, ok := reserve(l.shop, now); !ok {
		addrRes.CancelAt(now)
		keyRes.CancelAt(now)
		return false
	}
	return true
}

// reserve takes a token only if one is free right now. rate.Limiter has no way
// to ask without taking, so this reserves and hands it straight back when the
// answer is no — which is what Allow does internally, minus the ability to undo
// it later.
func reserve(l *rate.Limiter, now time.Time) (*rate.Reservation, bool) {
	r := l.ReserveN(now, 1)
	if !r.OK() || r.DelayFrom(now) > 0 {
		r.CancelAt(now)
		return nil, false
	}
	return r, true
}

// limiterFor gets or creates one bucket. Locked because get-then-add is not
// atomic, and two connections arriving together would otherwise each build a
// limiter with the second discarding the first's spent tokens.
func (l *keyedLimiter) limiterFor(c *lru.Cache[string, *rate.Limiter], key string, b bucket) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lim, ok := c.Get(key); ok {
		return lim
	}
	lim := rate.NewLimiter(b.r, b.burst)
	c.Add(key, lim)
	return lim
}

// sessionKey identifies a visitor. The server only offers publickey auth, so a
// session without one cannot reach here; the address is a fallback that keeps
// the limiter closed rather than handing an unkeyed session a free pass.
func sessionKey(s ssh.Session) string {
	if pk := s.PublicKey(); pk != nil {
		return gossh.FingerprintSHA256(pk)
	}
	return addrKey(s.RemoteAddr())
}

// addrKey reduces an address to the part that identifies a visitor.
//
// The port has to go: it is ephemeral, so keying on it would hand every
// reconnect a fresh bucket, and worse, churn a thousand single-use entries
// through the LRU and evict the addresses of everyone actually shopping.
//
// IPv6 collapses to its /64. A host is routinely handed a whole /64 and can pick
// a new address inside it for free, so keying on the full address would make
// address rotation as cheap as key rotation and leave this layer bounding
// nothing.
func addrKey(a net.Addr) string {
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		// Not TCP, so there may be no port to strip. Better a key that is too
		// specific than none at all.
		return a.String()
	}
	if v4 := tcp.IP.To4(); v4 != nil {
		return v4.String()
	}
	return tcp.IP.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
