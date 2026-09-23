package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
)

// The channel-disabled path (the last usable key is gone) reloads the local
// cache synchronously, so the channel leaves the enabled set as soon as the call
// returns.
//
// A plain key disable deliberately does not: the async watcher refresh covers
// it, which is enough as long as requests are spaced further apart than the
// refresh debounce. Reloading synchronously there would rebuild the whole cache
// a second time (the watcher notification already triggers one), so the extra
// cost buys nothing in that case.
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

// A plain key disable keeps the channel in service and leaves the cache refresh
// to the async path: in tests the async watcher is disabled, so the cached key
// set must still contain the disabled key until something reloads it.
func TestDisableAPIKey_PlainKeyDisableLeavesRefreshToAsyncPath(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	svc := newTestChannelService(client)

	ch := createTestChannelWithAPIKeys(t, client, ctx, "async-reload-channel", []string{"key1", "key2"})

	require.NoError(t, svc.enabledChannelsCache.Load(ctx, true))

	cached := svc.GetEnabledChannel(ch.ID)
	require.NotNil(t, cached)
	require.ElementsMatch(t, []string{"key1", "key2"}, cached.cachedEnabledAPIKeys)

	require.NoError(t, svc.DisableAPIKey(ctx, ch.ID, "key1", 401, "test"))

	// The channel stays in service, and the cache is refreshed by the async
	// watcher rather than inline.
	cached = svc.GetEnabledChannel(ch.ID)
	require.NotNil(t, cached)
	require.ElementsMatch(t, []string{"key1", "key2"}, cached.cachedEnabledAPIKeys)

	// Once something does reload the cache, the disabled key is gone.
	require.NoError(t, svc.enabledChannelsCache.Load(ctx, true))

	cached = svc.GetEnabledChannel(ch.ID)
	require.NotNil(t, cached)
	require.Equal(t, []string{"key2"}, cached.cachedEnabledAPIKeys)
}
