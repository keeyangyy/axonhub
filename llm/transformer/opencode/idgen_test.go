package opencode

import (
	"fmt"
	"regexp"
	"testing"
)

// Upstream Console validates ID shape: prefix + 12 hex + 14 base62 = 30 chars.
var idPattern = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$|^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

func TestGenSessionIDShape(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := genSessionID()
		if len(id) != 30 {
			t.Fatalf("session id %q: len = %d, want 30", id, len(id))
		}
		if !idPattern.MatchString(id) {
			t.Fatalf("session id %q does not match expected shape", id)
		}
	}
}

func TestGenRequestIDShape(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := genRequestID()
		if len(id) != 30 {
			t.Fatalf("request id %q: len = %d, want 30", id, len(id))
		}
		if !idPattern.MatchString(id) {
			t.Fatalf("request id %q does not match expected shape", id)
		}
	}
}

func TestGenIDsUnique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := genRequestID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = struct{}{}
	}
}

// Deterministic counter tests via the nowMillis clock seam. These mutate the
// package-level clock, so they must not run in parallel with other tests.
func TestGenRequestIDCounterIncrements(t *testing.T) {
	setClock(t, 1789000000000)

	id1 := genRequestID()
	id2 := genRequestID()

	// Same millisecond: timestamp hex part identical (first 9 of 12 hex
	// chars; the last 3 hold the counter), counter increments by 1.
	if id1[4:13] != id2[4:13] {
		t.Fatalf("timestamp hex changed within one ms: %s vs %s", id1[4:13], id2[4:13])
	}
	if ctr1, ctr2 := decodeCtr(id1), decodeCtr(id2); ctr2 != ctr1+1 {
		t.Fatalf("counter did not increment: %d -> %d", ctr1, ctr2)
	}

	// Millisecond rollover: counter resets to 1 and the hex part advances.
	setClock(t, 1789000000001)
	id3 := genRequestID()
	if c := decodeCtr(id3); c != 1 {
		t.Fatalf("counter did not reset after rollover: %d", c)
	}
	if id3[4:13] == id1[4:13] {
		t.Fatalf("timestamp hex did not advance after rollover: %s", id3[4:13])
	}
}

// Live-clock sanity check: two IDs generated back-to-back must differ and the
// packed 48-bit value must be monotonically non-decreasing (timestamp or
// counter advances; it never goes backwards within one process).
func TestGenRequestIDMonotonic(t *testing.T) {
	id1 := genRequestID()
	id2 := genRequestID()
	if id1 == id2 {
		t.Fatal("back-to-back ids are identical")
	}
	if decodePacked(id2) < decodePacked(id1) {
		t.Fatalf("packed value went backwards: %d -> %d", decodePacked(id1), decodePacked(id2))
	}
}

func setClock(t *testing.T, ms int64) {
	t.Helper()
	prev := nowMillis
	nowMillis = func() int64 { return ms }
	t.Cleanup(func() { nowMillis = prev })
}

func decodePacked(id string) int64 {
	var v int64
	for i := 0; i < 6; i++ {
		v = v<<8 | int64(hexVal(id[4+i*2]))<<4 | int64(hexVal(id[5+i*2]))
	}
	return v
}

func decodeCtr(id string) int64 { return decodePacked(id) & 0xfff }

// Session IDs are bitwise-inverted before packing; after inversion the top
// bits are no longer timestamp-only, so just verify decodability shape.
func TestGenSessionIDInversion(t *testing.T) {
	id := genSessionID()
	if id[:4] != "ses_" {
		t.Fatalf("prefix = %q, want ses_", id[:4])
	}
	// Inverted value's 12 hex chars must not be all f or all 0 in practice.
	hexPart := id[4:16]
	if hexPart == "000000000000" || hexPart == "ffffffffffff" {
		t.Fatalf("session hex part %q looks degenerate", hexPart)
	}
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	panic(fmt.Sprintf("invalid hex char %q", c))
}
