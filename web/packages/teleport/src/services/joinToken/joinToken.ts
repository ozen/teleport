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

import { makeLabelMapOfStrArrs } from '../agents/make';

import makeJoinToken from './makeJoinToken';
import { JoinToken, JoinRule, JoinTokenRequest } from './types';

class JoinTokenService {
  fetchJoinToken(
    req: JoinTokenRequest,
    signal: AbortSignal = null
  ): Promise<JoinToken> {
    return api
      .post(
        cfg.getJoinTokenUrl(),
        {
          roles: req.roles,
          join_method: req.method || 'token',
          allow: makeAllowField(req.rules || []),
          suggested_agent_matcher_labels: makeLabelMapOfStrArrs(
            req.suggestedAgentMatcherLabels
          ),
        },
        signal
      )
      .then(makeJoinToken);
  }
}

function makeAllowField(rules: JoinRule[] = []) {
  return rules.map(rule => ({
    aws_account: rule.awsAccountId,
    aws_arn: rule.awsArn,
  }));
}

export default JoinTokenService;
