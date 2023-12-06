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

import React from 'react';
import { render, screen, waitFor } from 'design/utils/testing';
import { AttemptStatus } from 'shared/hooks/useAsync';

import {
  makeLoggedInUser,
  makeRootCluster,
  makeServer,
} from 'teleterm/services/tshd/testHelpers';
import { MockAppContextProvider } from 'teleterm/ui/fixtures/MockAppContextProvider';
import { MockWorkspaceContextProvider } from 'teleterm/ui/fixtures/MockWorkspaceContextProvider';
import { MockAppContext } from 'teleterm/ui/fixtures/mocks';
import Logger, { NullService } from 'teleterm/logger';
import * as useResourcesContext from 'teleterm/ui/DocumentCluster/resourcesContext';

import * as connectMyComputerContext from '../connectMyComputerContext';

import { Setup } from './Setup';

beforeAll(() => {
  Logger.init(new NullService());
});

beforeEach(() => {
  jest.restoreAllMocks();
});

describe('setup of DocumentConnectMyComputer', () => {
  const tests: Array<{
    name: string;
    expectedStatus: AttemptStatus;
    mockAppContext?: (appContext: MockAppContext) => void;
  }> = [
    {
      name: 'ignores access denied errors from deleting the token',
      expectedStatus: 'success',
      // TODO(ravicious): In the future, it's probably better to make default mocks set up a happy
      // path and then use mockAppContext to reset any mocks that should behave differently for this
      // particular test.
      mockAppContext: appContext => {
        jest
          .spyOn(appContext.connectMyComputerService, 'deleteToken')
          .mockRejectedValue(new Error('7 PERMISSION_DENIED: access denied'));
      },
    },
    {
      name: 'does not ignore other errors when deleting the token',
      expectedStatus: 'error',
      mockAppContext: appContext => {
        jest
          .spyOn(appContext.connectMyComputerService, 'deleteToken')
          .mockRejectedValue(new Error('unknown error'));
      },
    },
  ];
  test.each(tests)('$name', async ({ expectedStatus, mockAppContext }) => {
    const { appContext, elementToRender } = setupAppContext();
    mockAppContext?.(appContext);

    render(elementToRender);

    screen.getByText('Connect').click();

    const step = await screen.findByTestId('Joining the cluster');

    await waitFor(
      () => expect(step).toHaveAttribute('data-teststatus', expectedStatus),
      // This makes debugging easier, as the error output will show the DOM for this step only.
      { container: step }
    );
  });

  it('calls requestResourcesRefresh after setup is done', async () => {
    const mockResourcesContext = {
      requestResourcesRefresh: jest.fn(),
      onResourcesRefreshRequest: jest.fn(),
    };
    jest
      .spyOn(useResourcesContext, 'useResourcesContext')
      .mockImplementation(() => mockResourcesContext);

    const { elementToRender } = setupAppContext();

    render(elementToRender);

    // Start the setup.
    screen.getByText('Connect').click();

    // Wait for the setup to finish.
    const step = await screen.findByTestId('Joining the cluster');
    await waitFor(
      () => expect(step).toHaveAttribute('data-teststatus', 'success'),
      { container: step }
    );

    expect(mockResourcesContext.requestResourcesRefresh).toHaveBeenCalledTimes(
      1
    );
  });

  it('attempts to remove the agent binary before retrying', async () => {
    const { elementToRender, appContext } = setupAppContext();

    jest
      .spyOn(appContext.connectMyComputerService, 'waitForNodeToJoin')
      .mockReset() // Reset mocks set by setupAppContext.
      .mockRejectedValueOnce(new Error('Something went wrong'))
      .mockResolvedValueOnce(makeServer());

    jest.spyOn(
      appContext.mainProcessClient,
      'tryRemoveConnectMyComputerAgentBinary'
    );

    render(elementToRender);

    // Start the setup and wait for the last step to fail.
    screen.getByText('Connect').click();
    const step = await screen.findByTestId('Joining the cluster');
    await waitFor(
      () => expect(step).toHaveAttribute('data-teststatus', 'error'),
      { container: step }
    );

    // Retry the setup and wait for the last step to succeed.
    screen.getByText('Retry').click();
    await waitFor(
      () => expect(step).toHaveAttribute('data-teststatus', 'success'),
      { container: step }
    );

    expect(
      appContext.mainProcessClient.tryRemoveConnectMyComputerAgentBinary
    ).toHaveBeenCalledTimes(1);
  });
});

function setupAppContext(): {
  elementToRender: React.ReactElement;
  appContext: MockAppContext;
} {
  const cluster = makeRootCluster({
    loggedInUser: makeLoggedInUser({
      acl: {
        tokens: {
          create: true,
          list: true,
          read: true,
          edit: true,
          pb_delete: true,
          use: true,
        },
      },
    }),
  });
  const appContext = new MockAppContext({
    appVersion: cluster.proxyVersion,
  });
  appContext.clustersService.state.clusters.set(cluster.uri, cluster);
  appContext.workspacesService.setState(draftState => {
    draftState.rootClusterUri = cluster.uri;
    draftState.workspaces[cluster.uri] = {
      localClusterUri: cluster.uri,
      documents: [],
      location: undefined,
      accessRequests: undefined,
    };
  });

  jest
    .spyOn(appContext.mainProcessClient, 'isAgentConfigFileCreated')
    .mockResolvedValue(false);
  jest
    .spyOn(appContext.connectMyComputerService, 'createRole')
    .mockResolvedValue({ certsReloaded: false });
  jest
    .spyOn(appContext.connectMyComputerService, 'createAgentConfigFile')
    .mockResolvedValue({ token: '1234' });
  jest
    .spyOn(appContext.connectMyComputerService, 'runAgent')
    .mockResolvedValue();
  jest
    .spyOn(appContext.connectMyComputerService, 'waitForNodeToJoin')
    .mockResolvedValue(makeServer());

  const elementToRender = (
    <MockAppContextProvider appContext={appContext}>
      <MockWorkspaceContextProvider rootClusterUri={cluster.uri}>
        <useResourcesContext.ResourcesContextProvider>
          <connectMyComputerContext.ConnectMyComputerContextProvider
            rootClusterUri={cluster.uri}
          >
            <Setup updateDocumentStatus={() => {}} />
          </connectMyComputerContext.ConnectMyComputerContextProvider>
        </useResourcesContext.ResourcesContextProvider>
      </MockWorkspaceContextProvider>
    </MockAppContextProvider>
  );

  return { elementToRender, appContext };
}
