package opencode

import (
	"strings"
	"testing"
	"time"
)

// These tests mutate the package-level session state and clock seam, so they
// must not run in parallel with each other.
func resetSession(t *testing.T) {
	t.Helper()
	sesMu.Lock()
	sesID, sesAt, sesLast = "", 0, 0
	sesMu.Unlock()
}

func TestStickySessionIDStableWithinIdleWindow(t *testing.T) {
	resetSession(t)

	setClock(t, 1789000000000)
	id1 := stickySessionID()

	// 10 minutes later, still within the 30 min idle window → same session.
	setClock(t, 1789000000000+10*60*1000)
	id2 := stickySessionID()

	if id1 != id2 {
		t.Fatalf("session changed within idle window:\n %s\n %s", id1, id2)
	}
	if id1[:4] != "ses_" || !idPattern.MatchString(id1) {
		t.Fatalf("session id %q has wrong shape", id1)
	}
}

func TestStickySessionIDRotatesAfterIdle(t *testing.T) {
	resetSession(t)

	setClock(t, 1789000000000)
	id1 := stickySessionID()

	// 31 minutes later → idle window exceeded → new session.
	setClock(t, 1789000000000+31*60*1000)
	id2 := stickySessionID()

	if id1 == id2 {
		t.Fatal("session was not rotated after idle timeout")
	}
	if strings.HasSuffix(id1, id2[len(id2)-14:]) && id1[4:16] == id2[4:16] {
		t.Fatalf("rotated session reuses packed time part: %s vs %s", id1, id2)
	}
}

func TestStickySessionIDHardCap(t *testing.T) {
	resetSession(t)

	setClock(t, 1789000000000)
	id1 := stickySessionID()

	// Constantly active up to (but excluding) the 6h cap → same session.
	step := int64(10 * 60 * 1000) // use every 10 minutes
	for ms := int64(1); ms*step < int64(sessionMaxAgeTTL/time.Millisecond); ms++ {
		setClock(t, 1789000000000+ms*step)
		idN := stickySessionID()
		if idN != id1 {
			t.Fatalf("session changed before hard cap at step %d", ms)
		}
	}

	// One more use beyond the cap → must rotate even without idleness.
	setClock(t, 1789000000000+int64(sessionMaxAgeTTL/time.Millisecond)+step)
	id2 := stickySessionID()
	if id1 == id2 {
		t.Fatal("session was not rotated after hard cap despite activity")
	}
}

func TestStickySessionIDConcurrent(t *testing.T) {
	resetSession(t)
	setClock(t, 1789000000000)

	const goroutines = 16
	ids := make(chan string, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			// Concurrent calls inside one idle window must all see the SAME
			// session, and the counter logic in idgen must not race (checked
			// by -race in the package test run).
			ids <- stickySessionID()
		}()
	}
	seen := map[string]struct{}{}
	for i := 0; i < goroutines; i++ {
		seen[<-ids] = struct{}{}
	}
	if len(seen) != 1 {
		t.Fatalf("concurrent calls produced %d distinct sessions, want 1", len(seen))
	}
}
