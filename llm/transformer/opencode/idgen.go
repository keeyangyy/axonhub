package opencode

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"
)

// OpenCode CLI free-tier requests must carry client-generated session/request IDs
// (X-Opencode-Session / X-Opencode-Request). The upstream Console validates their
// shape (prefix + 12 hex chars + 14 base62 chars); the CLI builds them from the
// current millisecond timestamp so IDs are fresh and unique per call. We mirror
// that algorithm so proxied requests keep passing even if the upstream later
// starts checking freshness or uniqueness.

const idChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var (
	idMu   sync.Mutex
	idLast int64
	idCtr  int64

	// nowMillis is a seam for tests to inject a fixed clock.
	nowMillis = func() int64 { return time.Now().UnixMilli() }
)

// genOpencodeID mirrors the OpenCode CLI ID generator.
// v = millisecond timestamp<<12 | per-millisecond counter, packed to the low
// 48 bits as 12 hex chars; session IDs additionally bitwise-invert v so the
// two ID spaces stay disjoint. 14 random base62 chars are appended.
func genOpencodeID(invert bool) string {
	idMu.Lock()
	now := nowMillis()
	if now != idLast {
		idCtr = 1
		idLast = now
	} else {
		idCtr++
	}
	ctr := idCtr
	idMu.Unlock()

	v := now<<12 | ctr
	if invert {
		v = ^v
	}

	var b strings.Builder
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&b, "%02x", byte(v>>(40-8*i)))
	}

	raw := make([]byte, 14)
	_, _ = rand.Read(raw)
	for _, c := range raw {
		b.WriteByte(idChars[int(c)%len(idChars)])
	}

	return b.String()
}

// genSessionID returns a ses_-prefixed OpenCode session ID.
func genSessionID() string { return "ses_" + genOpencodeID(true) }

// genRequestID returns a msg_-prefixed OpenCode request ID.
func genRequestID() string { return "msg_" + genOpencodeID(false) }
