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

import { NotificationsService } from 'teleterm/ui/services/notifications';
import { UsageService } from 'teleterm/ui/services/usage';
import { MainProcessClient } from 'teleterm/mainProcess/types';
import {
  makeDatabaseGateway,
  makeKubeGateway,
  makeRootCluster,
  makeLeafCluster,
} from 'teleterm/services/tshd/testHelpers';

import { ClustersService } from './clustersService';

import type * as uri from 'teleterm/ui/uri';
import type * as tsh from 'teleterm/services/tshd/types';

jest.mock('teleterm/ui/services/notifications');
jest.mock('teleterm/ui/services/usage');

const clusterUri: uri.RootClusterUri = '/clusters/test';

const clusterMock = makeRootCluster({
  uri: clusterUri,
  name: 'Test',
  proxyHost: 'localhost:3080',
});

const leafClusterMock = makeLeafCluster({
  uri: `${clusterUri}/leaves/test2`,
  name: 'Leaf',
});

const gatewayMock = makeDatabaseGateway({
  uri: '/gateways/gatewayTestUri',
  targetUri: `${clusterUri}/dbs/databaseTestUri`,
});

const NotificationsServiceMock = NotificationsService as jest.MockedClass<
  typeof NotificationsService
>;
const UsageServiceMock = UsageService as jest.MockedClass<typeof UsageService>;

function createService(client: Partial<tsh.TshClient>): ClustersService {
  return new ClustersService(
    client as tsh.TshClient,
    {
      removeKubeConfig: jest.fn().mockResolvedValueOnce(undefined),
    } as unknown as MainProcessClient,
    new NotificationsServiceMock(),
    new UsageServiceMock(undefined, undefined, undefined, undefined, undefined)
  );
}

function getClientMocks(): Partial<tsh.TshClient> {
  return {
    loginLocal: jest.fn().mockResolvedValueOnce(undefined),
    logout: jest.fn().mockResolvedValueOnce(undefined),
    addRootCluster: jest.fn().mockResolvedValueOnce(clusterMock),
    removeCluster: jest.fn().mockResolvedValueOnce(undefined),
    getCluster: jest.fn().mockResolvedValueOnce(clusterMock),
    listLeafClusters: jest.fn().mockResolvedValueOnce([leafClusterMock]),
    listGateways: jest.fn().mockResolvedValueOnce([gatewayMock]),
    createGateway: jest.fn().mockResolvedValueOnce(gatewayMock),
    removeGateway: jest.fn().mockResolvedValueOnce(undefined),
  };
}

test('add cluster', async () => {
  const { addRootCluster } = getClientMocks();
  const service = createService({
    addRootCluster,
  });

  await service.addRootCluster(clusterUri);

  expect(addRootCluster).toHaveBeenCalledWith(clusterUri);
  expect(service.state.clusters).toStrictEqual(
    new Map([[clusterUri, clusterMock]])
  );
});

test('remove cluster', async () => {
  const { removeGateway } = getClientMocks();
  const service = createService({ removeGateway });
  const gatewayFromRootCluster = makeDatabaseGateway({
    uri: '/gateways/1',
    targetUri: `${clusterMock.uri}/dbs/foo`,
  });
  const gatewayFromLeafCluster = makeDatabaseGateway({
    uri: '/gateways/2',
    targetUri: `${leafClusterMock.uri}/dbs/foo`,
  });
  const gatewayFromOtherCluster = makeDatabaseGateway({
    uri: '/gateways/3',
    targetUri: `/clusters/bogus-cluster/dbs/foo`,
  });

  service.setState(draftState => {
    draftState.clusters = new Map([
      [clusterMock.uri, clusterMock],
      [leafClusterMock.uri, leafClusterMock],
    ]);
    draftState.gateways = new Map([
      [gatewayFromRootCluster.uri, gatewayFromRootCluster],
      [gatewayFromLeafCluster.uri, gatewayFromLeafCluster],
      [gatewayFromOtherCluster.uri, gatewayFromOtherCluster],
    ]);
  });

  await service.removeClusterAndResources(clusterUri);

  expect(service.findCluster(clusterUri)).toBeUndefined();
  expect(service.findCluster(leafClusterMock.uri)).toBeUndefined();
  expect(service.state.gateways).toEqual(
    new Map([[gatewayFromOtherCluster.uri, gatewayFromOtherCluster]])
  );

  expect(removeGateway).toHaveBeenCalledWith(gatewayFromRootCluster.uri);
  expect(removeGateway).toHaveBeenCalledWith(gatewayFromLeafCluster.uri);
  expect(removeGateway).not.toHaveBeenCalledWith(gatewayFromOtherCluster.uri);
});

test('sync root cluster', async () => {
  const { getCluster, listLeafClusters } = getClientMocks();
  const service = createService({
    getCluster,
    listLeafClusters,
  });

  await service.syncRootClusterAndCatchErrors(clusterUri);

  expect(service.findCluster(clusterUri)).toStrictEqual(clusterMock);
  expect(service.findCluster(leafClusterMock.uri)).toStrictEqual(
    leafClusterMock
  );
  expect(listLeafClusters).toHaveBeenCalledWith(clusterUri);
});

test('login into cluster and sync cluster', async () => {
  const client = getClientMocks();
  const service = createService(client);
  const loginParams = {
    kind: 'local' as const,
    clusterUri,
    username: 'admin',
    password: 'admin',
    token: '1234',
  };

  await service.loginLocal(loginParams, undefined);

  expect(client.loginLocal).toHaveBeenCalledWith(loginParams, undefined);
  expect(service.findCluster(clusterUri).connected).toBe(true);
});

test('logout from cluster', async () => {
  const { logout, removeCluster } = getClientMocks();
  const service = createService({
    logout,
    removeCluster,
    getCluster: () => Promise.resolve({ ...clusterMock, connected: false }),
  });
  service.setState(draftState => {
    draftState.clusters = new Map([
      [clusterMock.uri, clusterMock],
      [leafClusterMock.uri, leafClusterMock],
    ]);
  });

  await service.logout(clusterUri);

  expect(logout).toHaveBeenCalledWith(clusterUri);
  expect(removeCluster).toHaveBeenCalledWith(clusterUri);
  expect(service.findCluster(clusterMock.uri).connected).toBe(false);
  expect(service.findCluster(leafClusterMock.uri).connected).toBe(false);
});

test('create a gateway', async () => {
  const { createGateway } = getClientMocks();
  const service = createService({
    createGateway,
  });
  const targetUri = '/clusters/foo/dbs/testId';
  const port = '2000';
  const user = 'alice';

  await service.createGateway({ targetUri, port, user });

  expect(createGateway).toHaveBeenCalledWith({ targetUri, port, user });
  expect(service.state.gateways).toStrictEqual(
    new Map([[gatewayMock.uri, gatewayMock]])
  );
});

test('remove a gateway', async () => {
  const { removeGateway } = getClientMocks();
  const service = createService({
    removeGateway,
  });
  const gatewayUri = '/gateways/gatewayUri';

  await service.removeGateway(gatewayUri);

  expect(removeGateway).toHaveBeenCalledWith(gatewayUri);
  expect(service.findGateway(gatewayUri)).toBeUndefined();
});

test('remove a kube gateway', async () => {
  const { removeGateway } = getClientMocks();
  const service = createService({
    removeGateway,
  });
  const kubeGatewayMock = makeKubeGateway({
    uri: '/gateways/gatewayTestUri',
    targetUri: `${clusterUri}/kubes/testKubeId`,
  });

  service.setState(draftState => {
    draftState.gateways = new Map([[kubeGatewayMock.uri, kubeGatewayMock]]);
  });

  await service.removeKubeGateway(kubeGatewayMock.targetUri as uri.KubeUri);
  expect(removeGateway).toHaveBeenCalledTimes(1);
  expect(removeGateway).toHaveBeenCalledWith(kubeGatewayMock.uri);
  expect(service.findGateway(kubeGatewayMock.uri)).toBeUndefined();

  // Calling it again should not increase mock calls.
  await service.removeKubeGateway(kubeGatewayMock.targetUri as uri.KubeUri);
  expect(removeGateway).toHaveBeenCalledTimes(1);
});

test('sync gateways', async () => {
  const { listGateways } = getClientMocks();
  const service = createService({
    listGateways,
  });

  await service.syncGatewaysAndCatchErrors();

  expect(service.state.gateways).toStrictEqual(
    new Map([[gatewayMock.uri, gatewayMock]])
  );
  expect(listGateways).toHaveBeenCalledWith();
});

test('find root cluster by resource URI', () => {
  const service = createService({});
  service.setState(draftState => {
    draftState.clusters = new Map([
      [clusterMock.uri, clusterMock],
      [leafClusterMock.uri, leafClusterMock],
    ]);
  });

  const foundClusters = service.findClusterByResource(
    `${clusterUri}/servers/foo`
  );

  expect(foundClusters).toStrictEqual(clusterMock);
});

test('find leaf cluster by resource URI', () => {
  const service = createService({});
  service.setState(draftState => {
    draftState.clusters = new Map([
      [clusterMock.uri, clusterMock],
      [leafClusterMock.uri, leafClusterMock],
    ]);
  });

  const foundClusters = service.findClusterByResource(
    `${leafClusterMock.uri}/servers/foo`
  );

  expect(foundClusters).toStrictEqual(leafClusterMock);
});
