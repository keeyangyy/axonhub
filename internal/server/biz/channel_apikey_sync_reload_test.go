package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
)

// TestDisableAPIKey_RefreshesEnabledChannelCacheSynchronously pins the fork
// behaviour: disabling a key must be visible in the in-memory channel cache as
// soon as DisableAPIKey returns.
//
// The async watcher notification is a no-op in tests (main_test.go sets
// asyncReloadDisabled), so this test only passes because DisableAPIKey reloads
// the cache synchronously. Without that reload the next request still selects
// the key that was just disabled, which makes a "disable after N failures" rule
// take effect one request late.
func TestDisableAPIKey_RefreshesEnabledChannelCacheSynchronously(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	svc := newTestChannelService(client)

	ch := createTestChannelWithAPIKeys(t, client, ctx, "sync-reload-channel", []string{"key1", "key2"})

	// Warm the cache the way a running server would.
	require.NoError(t, svc.enabledChannelsCache.Load(ctx, true))

	cached := svc.GetEnabledChannel(ch.ID)
	require.NotNil(t, cached)
	require.ElementsMatch(t, []string{"key1", "key2"}, cached.cachedEnabledAPIKeys)

	require.NoError(t, svc.DisableAPIKey(ctx, ch.ID, "key1", 401, "test"))

	// No explicit reload in between: the cache must already know about it.
	cached = svc.GetEnabledChannel(ch.ID)
	require.NotNil(t, cached)
	require.Equal(t, []string{"key2"}, cached.cachedEnabledAPIKeys)
	require.True(t, cached.IsAPIKeyDisabled("key1"))
}

// The channel-disabled path (the last usable key is gone) must keep working the
// way it did before: the channel leaves the enabled set as soon as the call
// returns.
func TestDisableAPIKey_RefreshesCacheWhenChannelGoesOutOfService(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	svc := newTestChannelService(client)

	ch := createTestChannelWithAPIKeys(t, client, ctx, "sync-reload-single-key", []string{"only-key"})

	require.NoError(t, svc.enabledChannelsCache.Load(ctx, true))
	require.NotNil(t, svc.GetEnabledChannel(ch.ID))

	require.NoError(t, svc.DisableAPIKey(ctx, ch.ID, "only-key", 401, "test"))

	require.Nil(t, svc.GetEnabledChannel(ch.ID))
}

// A disabled key stays disabled: re-running the disable must not resurrect it in
// the cache (the early return skips the reload, so the cache must already agree).
func TestDisableAPIKey_IdempotentDisableKeepsCacheConsistent(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	svc := newTestChannelService(client)

	ch := createTestChannelWithAPIKeys(t, client, ctx, "sync-reload-idempotent", []string{"key1", "key2"})

	require.NoError(t, svc.enabledChannelsCache.Load(ctx, true))

	require.NoError(t, svc.DisableAPIKey(ctx, ch.ID, "key1", 401, "first"))
	require.NoError(t, svc.DisableAPIKey(ctx, ch.ID, "key1", 401, "second"))

	cached := svc.GetEnabledChannel(ch.ID)
	require.NotNil(t, cached)
	require.Equal(t, []string{"key2"}, cached.cachedEnabledAPIKeys)
}
