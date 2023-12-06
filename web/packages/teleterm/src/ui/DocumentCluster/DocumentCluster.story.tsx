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

import React, { useEffect } from 'react';
import styled from 'styled-components';

import AppContextProvider from 'teleterm/ui/appContextProvider';
import { MockAppContext } from 'teleterm/ui/fixtures/mocks';
import {
  createClusterServiceState,
  ClustersServiceState,
} from 'teleterm/ui/services/clusters';
import { routing } from 'teleterm/ui/uri';
import {
  makeLoggedInUser,
  makeRootCluster,
  makeServer,
  makeDatabase,
  makeKube,
} from 'teleterm/services/tshd/testHelpers';

import { ResourcesService } from 'teleterm/ui/services/resources';
import { MockWorkspaceContextProvider } from 'teleterm/ui/fixtures/MockWorkspaceContextProvider';
import { ConnectMyComputerContextProvider } from 'teleterm/ui/ConnectMyComputer';
import * as docTypes from 'teleterm/ui/services/workspacesService/documentsService/types';
import * as tsh from 'teleterm/services/tshd/types';
import { makeDocumentCluster } from 'teleterm/ui/services/workspacesService/documentsService/testHelpers';

import DocumentCluster from './DocumentCluster';
import { ResourcesContextProvider } from './resourcesContext';

export default {
  title: 'Teleterm/DocumentCluster',
};

const rootClusterDoc = makeDocumentCluster({
  clusterUri: '/clusters/localhost',
  uri: '/docs/123',
});

const leafClusterDoc = makeDocumentCluster({
  clusterUri: '/clusters/localhost/leaves/foo',
  uri: '/docs/456',
});

export const OnlineEmptyResourcesAndCanAddResourcesAndConnectComputer = () => {
  const state = createClusterServiceState();
  state.clusters.set(
    rootClusterDoc.clusterUri,
    makeRootCluster({
      uri: rootClusterDoc.clusterUri,
      loggedInUser: makeLoggedInUser({
        userType: tsh.UserType.USER_TYPE_LOCAL,
        acl: {
          tokens: {
            create: true,
            list: true,
            edit: true,
            pb_delete: true,
            read: true,
            use: true,
          },
        },
      }),
    })
  );

  return renderState({
    state,
    doc: rootClusterDoc,
    platform: 'darwin',
    listUnifiedResources: () =>
      Promise.resolve({
        resources: [],
        totalCount: 0,
        nextKey: '',
      }),
  });
};

export const OnlineEmptyResourcesAndCanAddResourcesButCannotConnectComputer =
  () => {
    const state = createClusterServiceState();
    state.clusters.set(
      rootClusterDoc.clusterUri,
      makeRootCluster({
        uri: rootClusterDoc.clusterUri,
        loggedInUser: makeLoggedInUser({
          userType: tsh.UserType.USER_TYPE_SSO,
          acl: {
            tokens: {
              create: true,
              list: true,
              edit: true,
              pb_delete: true,
              read: true,
              use: true,
            },
          },
        }),
      })
    );

    return renderState({
      state,
      doc: rootClusterDoc,
      platform: 'win32',
      listUnifiedResources: () =>
        Promise.resolve({
          resources: [],
          totalCount: 0,
          nextKey: '',
        }),
    });
  };

export const OnlineEmptyResourcesAndCannotAddResources = () => {
  const state = createClusterServiceState();
  state.clusters.set(
    rootClusterDoc.clusterUri,
    makeRootCluster({
      uri: rootClusterDoc.clusterUri,
      loggedInUser: makeLoggedInUser({
        acl: {
          tokens: {
            create: false,
            list: true,
            edit: true,
            pb_delete: true,
            read: true,
            use: true,
          },
        },
      }),
    })
  );

  return renderState({
    state,
    doc: rootClusterDoc,
    listUnifiedResources: () =>
      Promise.resolve({
        resources: [],
        totalCount: 0,
        nextKey: '',
      }),
  });
};

export const OnlineLoadingResources = () => {
  const state = createClusterServiceState();
  state.clusters.set(
    rootClusterDoc.clusterUri,
    makeRootCluster({
      uri: rootClusterDoc.clusterUri,
    })
  );

  let rejectPromise: (error: Error) => void;
  const promiseRejectedOnUnmount = new Promise<any>((resolve, reject) => {
    rejectPromise = reject;
  });

  useEffect(() => {
    return () => {
      rejectPromise(new Error('Aborted'));
    };
  }, [rejectPromise]);

  return renderState({
    state,
    doc: rootClusterDoc,
    listUnifiedResources: () => promiseRejectedOnUnmount,
  });
};

export const OnlineLoadedResources = () => {
  const state = createClusterServiceState();
  state.clusters.set(
    rootClusterDoc.clusterUri,
    makeRootCluster({
      uri: rootClusterDoc.clusterUri,
    })
  );

  return renderState({
    state,
    doc: rootClusterDoc,
    listUnifiedResources: () =>
      Promise.resolve({
        resources: [
          {
            kind: 'server',
            resource: makeServer(),
          },
          {
            kind: 'server',
            resource: makeServer({
              uri: '/clusters/foo/servers/1234',
              hostname: 'bar',
              tunnel: true,
            }),
          },
          { kind: 'database', resource: makeDatabase() },
          { kind: 'kube', resource: makeKube() },
        ],
        totalCount: 4,
        nextKey: '',
      }),
  });
};

export const OnlineErrorLoadingResources = () => {
  const state = createClusterServiceState();
  state.clusters.set(
    rootClusterDoc.clusterUri,
    makeRootCluster({
      uri: rootClusterDoc.clusterUri,
    })
  );

  return renderState({
    state,
    doc: rootClusterDoc,
    listUnifiedResources: () =>
      Promise.reject(new Error('Whoops, something went wrong, sorry!')),
  });
};

export const Offline = () => {
  const state = createClusterServiceState();
  state.clusters.set(
    rootClusterDoc.clusterUri,
    makeRootCluster({
      connected: false,
      uri: rootClusterDoc.clusterUri,
    })
  );

  return renderState({ state, doc: rootClusterDoc });
};

export const Notfound = () => {
  const state = createClusterServiceState();
  state.clusters.set(
    rootClusterDoc.clusterUri,
    makeRootCluster({
      uri: rootClusterDoc.clusterUri,
    })
  );
  return renderState({ state, doc: leafClusterDoc });
};

function renderState({
  state,
  doc,
  listUnifiedResources,
  platform = 'darwin',
}: {
  state: ClustersServiceState;
  doc: docTypes.DocumentCluster;
  listUnifiedResources?: ResourcesService['listUnifiedResources'];
  platform?: NodeJS.Platform;
  userType?: tsh.UserType;
}) {
  const appContext = new MockAppContext({ platform });
  appContext.clustersService.state = state;

  const rootClusterUri = routing.ensureRootClusterUri(doc.clusterUri);
  appContext.workspacesService.setState(draftState => {
    draftState.rootClusterUri = rootClusterUri;
    draftState.workspaces[rootClusterUri] = {
      localClusterUri: doc.clusterUri,
      documents: [doc],
      location: doc.uri,
      accessRequests: undefined,
    };
  });

  appContext.resourcesService.listUnifiedResources = (params, abortSignal) =>
    listUnifiedResources
      ? listUnifiedResources(params, abortSignal)
      : Promise.reject('No fetchServersPromise passed');

  return (
    <AppContextProvider value={appContext}>
      <MockWorkspaceContextProvider>
        <ResourcesContextProvider>
          <ConnectMyComputerContextProvider rootClusterUri={rootClusterUri}>
            <Wrapper>
              <DocumentCluster visible={true} doc={doc} />
            </Wrapper>
          </ConnectMyComputerContextProvider>
        </ResourcesContextProvider>
      </MockWorkspaceContextProvider>
    </AppContextProvider>
  );
}

const Wrapper = styled.div`
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  bottom: 0;
`;
