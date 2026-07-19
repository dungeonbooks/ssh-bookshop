package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// ephemeralStorage reports whether this filesystem survives a restart. When it
// does not, a missing SSH_HOST_KEY is fatal rather than a warning.
//
// Two triggers because either alone fails open: our Dockerfile sets
// EPHEMERAL_STORAGE, but a service built some other way would lose the guard, so
// RAILWAY_SERVICE_ID counts too. Unparseable means ephemeral: a needless refusal
// is a log line, a wrongly permitted boot changes the shop's identity.
func ephemeralStorage() bool {
	if v := strings.TrimSpace(os.Getenv("EPHEMERAL_STORAGE")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return true
		}
		return b
	}
	return os.Getenv("RAILWAY_SERVICE_ID") != ""
}

// hostKeyPEM returns the server's SSH host key as PEM, read from SSH_HOST_KEY.
// Returns nil, nil when unset, so the caller can fall back to a key on disk.
//
// Pinned rather than generated because clients pin it too: a key that changes
// each deploy greets returning visitors with REMOTE HOST IDENTIFICATION HAS
// CHANGED. A Railway volume would also persist it, but Railway will not mount
// one to two deployments at once, forcing downtime on every deploy and a single
// instance.
//
// Three forms, all the same key: raw PEM, that PEM base64'd, or with its
// newlines backslash-escaped. The latter two exist for places that mangle
// newlines, .env being read a line at a time.
func hostKeyPEM() ([]byte, error) {
	raw := strings.TrimSpace(env("SSH_HOST_KEY", ""))
	if raw == "" {
		return nil, nil
	}

	var pem []byte
	if strings.Contains(raw, "-----BEGIN") {
		// A PEM carried through a shell or a web form often arrives with its
		// newlines backslash-escaped, which no parser accepts.
		if !strings.Contains(raw, "\n") {
			raw = strings.ReplaceAll(raw, `\n`, "\n")
		}
		pem = []byte(raw)
	} else {
		dec, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("SSH_HOST_KEY is neither PEM nor base64: %w", err)
		}
		pem = dec
	}

	// openssh wants the final newline and will not parse the key without it.
	if !bytes.HasSuffix(pem, []byte("\n")) {
		pem = append(pem, '\n')
	}

	// Parse here rather than letting wish fail later: a bad key should stop the
	// boot with a message naming the variable, not surface as a refused
	// handshake once someone tries to shop.
	if _, err := gossh.ParsePrivateKey(pem); err != nil {
		return nil, fmt.Errorf("SSH_HOST_KEY is not a usable private key: %w", err)
	}
	return pem, nil
}
