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

import type { Participant, ParticipantMode } from 'teleport/services/session';

interface DocumentBase {
  id?: number;
  title?: string;
  clusterId?: string;
  url: string;
  kind: 'terminal' | 'nodes' | 'blank';
  created: Date;
}

export interface DocumentBlank extends DocumentBase {
  kind: 'blank';
}

export interface DocumentSsh extends DocumentBase {
  status: 'connected' | 'disconnected';
  kind: 'terminal';
  sid?: string;
  mode?: ParticipantMode;
  serverId: string;
  login: string;
}

export interface DocumentNodes extends DocumentBase {
  kind: 'nodes';
}

export type Document = DocumentNodes | DocumentSsh | DocumentBlank;

export type Parties = Record<string, Participant[]>;
