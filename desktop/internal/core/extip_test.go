package core

import (
	"errors"
	"fmt"
	"testing"
)

// The SOCKS layer flattens every dial failure into 0x04 "host unreachable",
// so the probe has to read the text to tell a reconnect from a dead tunnel.
func TestTransientDialError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"socks host unreachable", errors.New("socks connect tcp 127.0.0.1:2080->api.ipify.org:443: unknown error host unreachable"), true},
		{"connection refused", errors.New("socks connect tcp 127.0.0.1:2080->api.ipify.org:443: connect: connection refused"), true},
		{"wrapped host unreachable", fmt.Errorf("запрос: %w", errors.New("dial tcp: unknown error host unreachable")), true},
		{"timeout", errors.New("context deadline exceeded"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := transientDialError(c.err); got != c.want {
			t.Errorf("%s: transientDialError(%v) = %v, want %v", c.name, c.err, got, c.want)
		}
	}
}
