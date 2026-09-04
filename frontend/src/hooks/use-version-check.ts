import { useEffect, useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import { graphqlRequest } from '@/gql/graphql';
import { toast } from 'sonner';
import { useAuthStore } from '@/stores/authStore';
import i18n from '@/lib/i18n';
import { CHECK_FOR_UPDATE_QUERY, OFFICIAL_REPO, FORK_REPO, type VersionCheck } from '@/features/system/data/system';

const VERSION_CHECK_STORAGE_KEY = 'axonhub_dismissed_version';
const VERSION_CHECK_TIMESTAMP_KEY = 'axonhub_version_check_timestamp';
const VERSION_CHECK_INTERVAL = 10 * 60 * 1000; // 10 minutes in milliseconds

function useChannelCheck(repo: string, enabled: boolean) {
  return useQuery({
    queryKey: ['versionCheck', repo],
    queryFn: async () => {
      const data = await graphqlRequest<{ checkForUpdate: VersionCheck }>(CHECK_FOR_UPDATE_QUERY, {
        includeBeta: true,
        repo,
      });
      return data.checkForUpdate;
    },
    enabled,
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnMount: false,
    refetchOnReconnect: false,
  });
}

/**
 * Hook to check for new versions and show toast notification.
 * Checks both the official upstream and the fork channel, preferring the fork
 * (which is the channel the upgrade script downloads from).
 * Only shows to owners and only once per version.
 */
export function useVersionCheck() {
  const user = useAuthStore((state) => state.auth.user);
  const isOwner = user?.isOwner ?? false;

  const shouldCheckVersion = useCallback(() => {
    if (!isOwner) return false;

    const lastCheckTime = localStorage.getItem(VERSION_CHECK_TIMESTAMP_KEY);
    if (!lastCheckTime) return true;

    const timeSinceLastCheck = Date.now() - parseInt(lastCheckTime, 10);
    return timeSinceLastCheck >= VERSION_CHECK_INTERVAL;
  }, [isOwner]);

  const enabled = shouldCheckVersion();
  const official = useChannelCheck(OFFICIAL_REPO, enabled);
  const fork = useChannelCheck(FORK_REPO, enabled);

  // Store the timestamp after a successful check
  useEffect(() => {
    if (enabled && (official.isSuccess || fork.isSuccess)) {
      localStorage.setItem(VERSION_CHECK_TIMESTAMP_KEY, Date.now().toString());
    }
  }, [enabled, official.isSuccess, fork.isSuccess]);

  const showUpdateToast = useCallback((latestVersion: string, releaseUrl: string) => {
    toast.info(i18n.t('system.about.updateCheck.newVersionAvailable'), {
      description: `${i18n.t('system.about.updateCheck.latestVersion')}: ${latestVersion}`,
      duration: 10000,
      action: {
        label: i18n.t('system.about.updateCheck.viewRelease'),
        onClick: () => {
          window.open(releaseUrl, '_blank', 'noopener,noreferrer');
        },
      },
    });
  }, []);

  useEffect(() => {
    if (!isOwner) return;

    // Prefer the fork channel (matching the upgrade script), fall back to official.
    const preferred = fork.data?.hasUpdate ? fork.data : official.data?.hasUpdate ? official.data : null;
    if (!preferred) return;

    // Check if this version was already dismissed
    const dismissedVersion = localStorage.getItem(VERSION_CHECK_STORAGE_KEY);
    if (dismissedVersion === preferred.latestVersion) return;

    // Show toast and mark as shown
    showUpdateToast(preferred.latestVersion, preferred.releaseUrl);
    localStorage.setItem(VERSION_CHECK_STORAGE_KEY, preferred.latestVersion);
  }, [isOwner, official.data, fork.data, showUpdateToast]);
}
