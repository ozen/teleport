/**
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

import React, {
  createContext,
  useCallback,
  PropsWithChildren,
  useContext,
  useRef,
  useEffect,
  useState,
} from 'react';

import useAttempt from 'shared/hooks/useAttemptNext';

import { Indicator } from 'design';

import { StyledIndicator } from 'teleport/Main';

import * as service from 'teleport/services/userPreferences';
import cfg from 'teleport/config';

import { KeysEnum, storageService } from 'teleport/services/storageService';

import {
  deprecatedThemeToThemePreference,
  ThemePreference,
} from 'teleport/services/userPreferences/types';

import { makeDefaultUserPreferences } from 'teleport/services/userPreferences/userPreferences';

import type {
  UserClusterPreferences,
  UserPreferences,
} from 'teleport/services/userPreferences/types';

export interface UserContextValue {
  preferences: UserPreferences;
  updatePreferences: (preferences: Partial<UserPreferences>) => Promise<void>;
  updateClusterPinnedResources: (
    clusterId: string,
    pinnedResources: string[]
  ) => Promise<void>;
  getClusterPinnedResources: (clusterId: string) => Promise<string[]>;
}

export const UserContext = createContext<UserContextValue>(null);

export function useUser(): UserContextValue {
  return useContext(UserContext);
}

export function UserContextProvider(props: PropsWithChildren<unknown>) {
  const { attempt, run } = useAttempt('processing');
  // because we have to update cluster preferences with itself during the update
  // we useRef here to prevent infinite rerenders
  const clusterPreferences = useRef<Record<string, UserClusterPreferences>>({});

  const [preferences, setPreferences] = useState<UserPreferences>(
    makeDefaultUserPreferences()
  );

  const getClusterPinnedResources = useCallback(async (clusterId: string) => {
    if (clusterPreferences.current[clusterId]) {
      // we know that pinned resources is supported because we've already successfully
      // fetched their pinned resources once before
      window.localStorage.removeItem(KeysEnum.PINNED_RESOURCES_NOT_SUPPORTED);
      return clusterPreferences.current[clusterId].pinnedResources;
    }
    const prefs = await service.getUserClusterPreferences(clusterId);
    if (prefs) {
      clusterPreferences.current[clusterId] = prefs;
      return prefs.pinnedResources;
    }
    return null;
  }, []);

  const updateClusterPinnedResources = async (
    clusterId: string,
    pinnedResources: string[]
  ) => {
    if (!clusterPreferences.current[clusterId]) {
      clusterPreferences.current[clusterId] = { pinnedResources: [] };
    }
    clusterPreferences.current[clusterId].pinnedResources = pinnedResources;

    return service.updateUserClusterPreferences(clusterId, {
      ...preferences,
      clusterPreferences: clusterPreferences.current[clusterId],
    });
  };

  async function loadUserPreferences() {
    const storedPreferences = storageService.getUserPreferences();
    const theme = storageService.getDeprecatedThemePreference();

    try {
      const preferences = await service.getUserPreferences();
      clusterPreferences.current[cfg.proxyCluster] =
        preferences.clusterPreferences;
      if (!storedPreferences) {
        // there are no mirrored user preferences in local storage so this is the first time
        // the user has requested their preferences in this browser session

        // if there is a legacy theme preference, update the preferences with it and remove it
        if (theme) {
          preferences.theme = deprecatedThemeToThemePreference(theme);

          if (preferences.theme !== ThemePreference.Light) {
            // the light theme is the default, so only update the backend if it is not light
            updatePreferences(preferences);
          }

          storageService.clearDeprecatedThemePreference();
        }
      }

      setPreferences(preferences);
      storageService.setUserPreferences(preferences);
    } catch (err) {
      if (storedPreferences) {
        setPreferences(storedPreferences);

        return;
      }

      if (theme) {
        setPreferences({
          ...preferences,
          theme: deprecatedThemeToThemePreference(theme),
        });
      }
    }
  }

  function updatePreferences(newPreferences: Partial<UserPreferences>) {
    const nextPreferences = {
      ...preferences,
      ...newPreferences,
      assist: {
        ...preferences.assist,
        ...newPreferences.assist,
      },
      onboard: {
        ...preferences.onboard,
        ...newPreferences.onboard,
      },
      unifiedResourcePreferences: {
        ...preferences.unifiedResourcePreferences,
        ...newPreferences.unifiedResourcePreferences,
      },
      // updatePreferences only update the root cluster so we can only pass cluster
      // preferences from the root cluster
      clusterPreferences: clusterPreferences.current[cfg.proxyCluster],
    } as UserPreferences;
    setPreferences(nextPreferences);
    storageService.setUserPreferences(nextPreferences);

    return service.updateUserPreferences(nextPreferences);
  }

  useEffect(() => {
    function receiveMessage(event: StorageEvent) {
      if (!event.newValue || event.key !== KeysEnum.USER_PREFERENCES) {
        return;
      }

      setPreferences(JSON.parse(event.newValue));
    }

    storageService.subscribe(receiveMessage);

    return () => storageService.unsubscribe(receiveMessage);
  }, []);

  useEffect(() => {
    run(loadUserPreferences);
  }, []);

  if (attempt.status === 'processing') {
    return (
      <StyledIndicator>
        <Indicator />
      </StyledIndicator>
    );
  }

  return (
    <UserContext.Provider
      value={{
        preferences,
        updatePreferences,
        getClusterPinnedResources,
        updateClusterPinnedResources,
      }}
    >
      {props.children}
    </UserContext.Provider>
  );
}
