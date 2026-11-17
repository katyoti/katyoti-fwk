// SPDX-License-Identifier: Apache-2.0

package nats

import (
	"errors"
	"net"
	"testing"
	"time"
)

// listen opens a TCP listener on a random port and returns it with the
// address it accepted, so a dial can be told apart by where it lands.
func listen(t *testing.T) (net.Listener, string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, l.Addr().String()
}

// TestResolvingDialer_DialsWhatTheResolverNamesNow: the address nats-go asks
// for is the one the client was created with; the resolver names where the
// server IS, and that is where the dial lands. With nothing resolved (no
// lock file yet, an error), the given address is dialled — the reconnect
// loop keeps its own behaviour until there is something newer to follow.
func TestResolvingDialer_DialsWhatTheResolverNamesNow(t *testing.T) {
	oldL, oldAddr := listen(t)
	newL, newAddr := listen(t)
	accepted := func(l net.Listener) <-chan struct{} {
		ch := make(chan struct{}, 1)
		go func() {
			if c, err := l.Accept(); err == nil {
				_ = c.Close()
				ch <- struct{}{}
			}
		}()
		return ch
	}
	gotNew := accepted(newL)
	gotOld := accepted(oldL)

	current := "nats://" + newAddr
	d := &resolvingDialer{resolve: func() (string, error) { return current, nil }, timeout: time.Second}
	c, err := d.Dial("tcp", oldAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = c.Close()
	select {
	case <-gotNew:
	case <-time.After(2 * time.Second):
		t.Fatal("the dial did not land on the address the resolver named")
	}

	// Nothing resolved: the given address.
	for _, resolve := range []func() (string, error){
		func() (string, error) { return "", nil },
		func() (string, error) { return "", errors.New("no lock file") },
		func() (string, error) { return "::not a url", nil },
	} {
		d := &resolvingDialer{resolve: resolve, timeout: time.Second}
		c, err := d.Dial("tcp", oldAddr)
		if err != nil {
			t.Fatalf("dial with nothing resolved: %v", err)
		}
		_ = c.Close()
		select {
		case <-gotOld:
			gotOld = accepted(oldL)
		case <-time.After(2 * time.Second):
			t.Fatal("with nothing resolved the dial must land on the given address")
		}
	}

	// A server that is gone at the resolved address is a dial error, not a
	// silent fall-through to the old one: nats-go retries on its own clock.
	_ = newL.Close()
	current = "nats://" + newAddr
	d = &resolvingDialer{resolve: func() (string, error) { return current, nil }, timeout: 500 * time.Millisecond}
	if c, err := d.Dial("tcp", oldAddr); err == nil {
		_ = c.Close()
		t.Fatal("a refused resolved address must fail the dial, not fall through")
	}
}
