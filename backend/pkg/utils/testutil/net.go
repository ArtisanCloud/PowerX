package testutil

import (
	"net"
	"testing"
	"time"
)

// SkipIfNoLocalListener skips the test if binding to a local TCP port is not permitted.
func SkipIfNoLocalListener(t testing.TB) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local TCP listen not permitted: %v", err)
		return
	}
	_ = l.Close()
}

// SkipIfNoTCPDial skips the test if connecting to addr is not permitted.
func SkipIfNoTCPDial(t testing.TB, addr string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		t.Skipf("TCP dial not permitted for %s: %v", addr, err)
		return
	}
	_ = conn.Close()
}
