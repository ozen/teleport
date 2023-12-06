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

import { useEffect } from 'react';

import { useAsync } from 'shared/hooks/useAsync';

import { useAppContext } from 'teleterm/ui/appContextProvider';
import * as types from 'teleterm/ui/services/workspacesService';
import { useWorkspaceContext } from 'teleterm/ui/Documents';
import { retryWithRelogin } from 'teleterm/ui/utils';
import * as tshdGateway from 'teleterm/services/tshd/gateway';

export function useDocumentGateway(doc: types.DocumentGateway) {
  const ctx = useAppContext();
  const { documentsService: workspaceDocumentsService } = useWorkspaceContext();
  // The port to show as default in the input field in case creating a gateway fails.
  // This is typically the case if someone reopens the app and the port of the gateway is already
  // occupied.
  //
  // This needs a default value as otherwise React will complain about switching an uncontrolled
  // input to a controlled one once `doc.port` gets set. The backend will handle converting an empty
  // string to '0'.
  const defaultPort = doc.port || '';
  const gateway = ctx.clustersService.findGateway(doc.gatewayUri);
  const connected = !!gateway;

  const [connectAttempt, createGateway] = useAsync(async (port: string) => {
    const gw = await retryWithRelogin(ctx, doc.targetUri, () =>
      ctx.clustersService.createGateway({
        targetUri: doc.targetUri,
        port: port,
        user: doc.targetUser,
        subresource_name: doc.targetSubresourceName,
      })
    );

    workspaceDocumentsService.update(doc.uri, {
      gatewayUri: gw.uri,
      // Set the port on doc to match the one returned from the daemon. Teleterm doesn't let the
      // user provide a port for the gateway, so instead we have to let the daemon use a random
      // one.
      //
      // Setting it here makes it so that on app restart, Teleterm will restart the proxy with the
      // same port number.
      port: gw.localPort,
    });
    ctx.usageService.captureProtocolUse(doc.targetUri, 'db', doc.origin);
  });

  const [disconnectAttempt, disconnect] = useAsync(async () => {
    await ctx.clustersService.removeGateway(doc.gatewayUri);
    workspaceDocumentsService.close(doc.uri);
  });

  const [changeDbNameAttempt, changeDbName] = useAsync(async (name: string) => {
    const updatedGateway =
      await ctx.clustersService.setGatewayTargetSubresourceName(
        doc.gatewayUri,
        name
      );

    workspaceDocumentsService.update(doc.uri, {
      targetSubresourceName: updatedGateway.targetSubresourceName,
    });
  });

  const [changePortAttempt, changePort] = useAsync(async (port: string) => {
    const updatedGateway = await ctx.clustersService.setGatewayLocalPort(
      doc.gatewayUri,
      port
    );

    workspaceDocumentsService.update(doc.uri, {
      targetSubresourceName: updatedGateway.targetSubresourceName,
      port: updatedGateway.localPort,
    });
  });

  const runCliCommand = () => {
    const command = tshdGateway.getCliCommandArgv0(gateway.gatewayCliCommand);
    const title = `${command} · ${doc.targetUser}@${doc.targetName}`;

    const cliDoc = workspaceDocumentsService.createGatewayCliDocument({
      title,
      targetUri: doc.targetUri,
      targetUser: doc.targetUser,
      targetName: doc.targetName,
      targetProtocol: gateway.protocol,
    });
    workspaceDocumentsService.add(cliDoc);
    workspaceDocumentsService.setLocation(cliDoc.uri);
  };

  useEffect(
    function createGatewayOnMount() {
      // Since the user can close DocumentGateway without shutting down the gateway, it's possible
      // to open DocumentGateway while the gateway is already running. In that scenario, we must
      // not attempt to create a gateway.
      if (!gateway && connectAttempt.status === '') {
        createGateway(doc.port);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );

  return {
    gateway,
    defaultPort,
    disconnect,
    connected,
    reconnect: createGateway,
    connectAttempt,
    // TODO(ravicious): Show disconnectAttempt errors in UI.
    disconnectAttempt,
    runCliCommand,
    changeDbName,
    changeDbNameAttempt,
    changePort,
    changePortAttempt,
  };
}
