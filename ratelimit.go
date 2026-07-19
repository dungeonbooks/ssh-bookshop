package main

import (
	"net"
	"sync"

	"golang.org/x/time/rate"

	"charm.land/wish/v2/ratelimiter"
	"github.com/charmbracelet/ssh"
	lru "github.com/hashicorp/golang-lru/v2"
	gossh "golang.org/x/crypto/ssh"
)

// Connection limits: a blast door against a script opening sessions in a loop,
// not fairness between shoppers.
//
// wish's own limiter keys on remote IP, which is the right key almost anywhere
// and the wrong one here: Railway's TCP proxy is layer 4 with no PROXY protocol,
// so every visitor arrives from the same address and the whole shop shares a
// single bucket. A limit tight enough to stop one abuser would shut the shop for
// everyone, which is why these were loose.
//
// Keying on the SSH public key fingerprint restores a bucket per visitor. It
// does not stop a determined abuser, who can mint a fresh keypair for free, so
// the shop-wide ceiling stays underneath as the bound key rotation cannot
// escape. Per-key is for fairness; global is for survival.
const (
	keyRate  = 1 // sustained connections/sec for one public key
	keyBurst = 5 // a few quick reconnects are normal

	// Left exactly where the IP-keyed limit was, since on Railway that was
	// already the shop-wide figure. Per-key limiting is added on top rather
	// than traded against it, so nothing here loosens.
	shopRate  = 10
	shopBurst = 30

	connCache = 1024 // distinct fingerprints tracked
)

// keyedLimiter allows a session when both its own key's bucket and the shop-wide
// bucket have room.
type keyedLimiter struct {
	mu    sync.Mutex
	cache *lru.Cache[string, *rate.Limiter]
	shop  *rate.Limiter
}

func connectionLimiter() ratelimiter.RateLimiter {
	cache, err := lru.New[string, *rate.Limiter](connCache)
	if err != nil {
		// Only reachable by editing connCache to a non-positive value, which is
		// a programming error rather than a runtime condition. Failing here says
		// so; swallowing it returns a nil cache that panics on the first
		// connection instead, a long way from the cause.
		panic("connection limiter cache: " + err.Error())
	}
	return &keyedLimiter{
		cache: cache,
		shop:  rate.NewLimiter(shopRate, shopBurst),
	}
}

func (l *keyedLimiter) Allow(s ssh.Session) error {
	if !l.allow(sessionKey(s)) {
		return ratelimiter.ErrRateLimitExceeded
	}
	return nil
}

// allow is the decision without the session around it, so the buckets can be
// tested without standing up an SSH server.
func (l *keyedLimiter) allow(key string) bool {
	// Per-key first: it is the common rejection, and checking it first means a
	// noisy client spends its own tokens rather than the shop's.
	if !l.limiterFor(key).Allow() {
		return false
	}
	return l.shop.Allow()
}

// limiterFor gets or creates the bucket for one key. Locked because get-then-add
// is not atomic, and two connections arriving together would otherwise each
// build a limiter with the second discarding the first's spent tokens.
func (l *keyedLimiter) limiterFor(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lim, ok := l.cache.Get(key); ok {
		return lim
	}
	lim := rate.NewLimiter(keyRate, keyBurst)
	l.cache.Add(key, lim)
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

// addrKey reduces an address to the part that identifies a visitor. The port
// has to go: it is ephemeral, so keying on it would hand every reconnect a
// fresh bucket, and worse, churn a thousand single-use entries through the LRU
// and evict the fingerprints of everyone actually shopping.
func addrKey(a net.Addr) string {
	if tcp, ok := a.(*net.TCPAddr); ok {
		return tcp.IP.String()
	}
	// Not TCP, so there may be no port to strip. Better a key that is too
	// specific than none at all.
	return a.String()
}
