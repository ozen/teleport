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

import api from 'teleport/services/api';
import cfg from 'teleport/config';

import { makeConnectionDiagnostic } from './make';

import type {
  ConnectionDiagnostic,
  ConnectionDiagnosticRequest,
} from './types';

export const agentService = {
  createConnectionDiagnostic(
    req: ConnectionDiagnosticRequest
  ): Promise<ConnectionDiagnostic> {
    return api
      .post(cfg.getConnectionDiagnosticUrl(), {
        resource_kind: req.resourceKind,
        resource_name: req.resourceName,
        ssh_principal: req.sshPrincipal,
        kubernetes_namespace: req.kubeImpersonation?.namespace,
        kubernetes_impersonation: {
          kubernetes_user: req.kubeImpersonation?.user,
          kubernetes_groups: req.kubeImpersonation?.groups,
        },
        database_name: req.dbTester?.name,
        database_user: req.dbTester?.user,
        mfa_response: req.mfaAuthnResponse,
      })
      .then(makeConnectionDiagnostic);
  },
};
