package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// testKeyPEM makes a real ed25519 host key rather than pasting a fixture, so the
// test proves the parser accepts what ssh-keygen would actually produce.
func testKeyPEM(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blk, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(blk))
}

func TestHostKeyPEM(t *testing.T) {
	key := testKeyPEM(t)

	t.Run("unset falls back to disk", func(t *testing.T) {
		t.Setenv("SSH_HOST_KEY", "")
		got, err := hostKeyPEM()
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if got != nil {
			t.Errorf("got %d bytes, want nil so the caller uses the key on disk", len(got))
		}
	})

	t.Run("raw pem", func(t *testing.T) {
		t.Setenv("SSH_HOST_KEY", key)
		got, err := hostKeyPEM()
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if _, err := gossh.ParsePrivateKey(got); err != nil {
			t.Errorf("result does not parse: %v", err)
		}
	})

	t.Run("base64 pem, for .env which cannot hold newlines", func(t *testing.T) {
		t.Setenv("SSH_HOST_KEY", base64.StdEncoding.EncodeToString([]byte(key)))
		got, err := hostKeyPEM()
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if _, err := gossh.ParsePrivateKey(got); err != nil {
			t.Errorf("result does not parse: %v", err)
		}
	})

	t.Run("escaped newlines, as a shell or web form leaves them", func(t *testing.T) {
		t.Setenv("SSH_HOST_KEY", strings.ReplaceAll(key, "\n", `\n`))
		got, err := hostKeyPEM()
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if _, err := gossh.ParsePrivateKey(got); err != nil {
			t.Errorf("result does not parse: %v", err)
		}
	})

	t.Run("missing trailing newline", func(t *testing.T) {
		t.Setenv("SSH_HOST_KEY", strings.TrimRight(key, "\n"))
		got, err := hostKeyPEM()
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if _, err := gossh.ParsePrivateKey(got); err != nil {
			t.Errorf("result does not parse: %v", err)
		}
	})

	// The point of parsing at boot: a bad value must stop the process with a
	// message naming the variable, not become a refused handshake later.
	for _, tc := range []struct{ name, val string }{
		{"garbage", "not a key at all"},
		{"truncated pem", "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n"},
		{"base64 of something else", base64.StdEncoding.EncodeToString([]byte("hello"))},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			t.Setenv("SSH_HOST_KEY", tc.val)
			if _, err := hostKeyPEM(); err == nil {
				t.Error("err = nil, want a boot-stopping error")
			}
		})
	}
}

// TestHostKeyStableAcrossBoots is the property the whole thing exists for: the
// same variable must yield the same public key every time, or returning
// visitors get REMOTE HOST IDENTIFICATION HAS CHANGED after a deploy.
func TestHostKeyStableAcrossBoots(t *testing.T) {
	key := testKeyPEM(t)
	fingerprint := func() string {
		t.Helper()
		pem, err := hostKeyPEM()
		if err != nil {
			t.Fatal(err)
		}
		signer, err := gossh.ParsePrivateKey(pem)
		if err != nil {
			t.Fatal(err)
		}
		return gossh.FingerprintSHA256(signer.PublicKey())
	}

	t.Setenv("SSH_HOST_KEY", key)
	first := fingerprint()
	// Same key, differently encoded: still the same identity to a client.
	t.Setenv("SSH_HOST_KEY", base64.StdEncoding.EncodeToString([]byte(key)))
	if second := fingerprint(); second != first {
		t.Errorf("fingerprint changed across encodings: %s then %s", first, second)
	}
}

// TestEphemeralStorage pins the guard's trigger. Getting the default wrong in
// the permissive direction is the expensive one: the shop boots on a throwaway
// key and quietly changes identity on every deploy.
func TestEphemeralStorage(t *testing.T) {
	for _, tc := range []struct {
		name          string
		flag, railway string
		want          bool
	}{
		{"unset anywhere is local development", "", "", false},
		{"our image sets it", "1", "", true},
		{"true", "true", "", true},

		// The reason this is parsed rather than tested for emptiness: 0 and
		// false read as "off" to anyone setting them, and an emptiness check
		// would silently mean the opposite.
		{"0 disables", "0", "", false},
		{"false disables", "false", "", false},
		{"explicit off wins over railway", "false", "svc-123", false},

		// Railway injects RAILWAY_SERVICE_ID into every deployment however it
		// was built, so the guard survives a service that stops using our
		// Dockerfile.
		{"railway without the flag", "", "svc-123", true},

		// Unparseable is treated as on: a needless refusal is a log line, a
		// wrongly permitted boot changes the shop's identity.
		{"garbage fails closed", "yes-please", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EPHEMERAL_STORAGE", tc.flag)
			t.Setenv("RAILWAY_SERVICE_ID", tc.railway)
			if got := ephemeralStorage(); got != tc.want {
				t.Errorf("ephemeralStorage() = %v, want %v", got, tc.want)
			}
		})
	}
}
