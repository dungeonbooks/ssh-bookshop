package main

import (
	"errors"
	"testing"
)

// Whether the shop keeps a client after a rocky load decides whether it can
// sell at all, so the rule is pinned rather than left to the call site.
func TestUsable(t *testing.T) {
	boom := errors.New("square is down")
	for _, tc := range []struct {
		name   string
		priced int
		err    error
		want   bool
	}{
		{"a clean load", 7, nil, true},
		{"clean but nothing on the shelf", 0, nil, true},
		{"partly priced, so it can still sell those", 3, boom, true},
		{"priced nothing, so there is nothing to sell", 0, boom, false},
	} {
		if got := usable(tc.priced, tc.err); got != tc.want {
			t.Errorf("%s: usable(%d, %v) = %v, want %v", tc.name, tc.priced, tc.err, got, tc.want)
		}
	}
}

// Without a token there is no client to keep, and leaving a half-built one in
// sq would let checkout call an API it cannot authenticate to.
func TestLoadShopWithoutTokenLeavesSqNil(t *testing.T) {
	t.Setenv("SQUARE_ACCESS_TOKEN", "")
	before := sq
	t.Cleanup(func() { sq = before })

	priced, err := loadShop(catalog)
	if err == nil {
		t.Fatal("err = nil, want a missing-token error")
	}
	if priced != 0 {
		t.Errorf("priced = %d, want 0", priced)
	}
	if sq != before {
		t.Error("sq was set despite there being no token")
	}
}
