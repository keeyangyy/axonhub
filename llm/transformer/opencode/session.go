package opencode

import (
	"sync"
	"time"
)

// The free tier tolerates client-generated IDs, but a per-request session ID
// is an outlier pattern: a real OpenCode CLI keeps one ses_ ID alive for the
// whole conversation (ses:msg ≈ 1:N, sessions living minutes to hours), while
// 1:1 sessions that die instantly stand out in any aggregate analysis. Stick
// to one shared session ID and rotate it on inactivity or absolute age to
// mimic a natural conversation lifecycle. No per-key affinity on purpose:
// keys rotate frequently, and the shared ID behaves like one steady CLI user.

const (
	sessionIdleTTL   = 30 * time.Minute // new session after this much inactivity
	sessionMaxAgeTTL = 6 * time.Hour    // hard cap even if constantly active
)

var (
	sesMu   sync.Mutex
	sesID   string
	sesAt   int64 // creation time (ms, via nowMillis clock seam)
	sesLast int64 // last-used time (ms)
)

// stickySessionID returns the shared fallback session ID, rotating it when it
// has been idle longer than sessionIdleTTL or alive longer than
// sessionMaxAgeTTL. Uses the same clock seam (nowMillis) as idgen.go.
func stickySessionID() string {
	sesMu.Lock()
	defer sesMu.Unlock()

	now := nowMillis()
	idleMax := int64(sessionIdleTTL / time.Millisecond)
	ageMax := int64(sessionMaxAgeTTL / time.Millisecond)
	if sesID == "" || now-sesLast >= idleMax || now-sesAt >= ageMax {
		sesID = genSessionID()
		sesAt = now
	}
	sesLast = now
	return sesID
}
