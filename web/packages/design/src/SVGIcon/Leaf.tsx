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

import { SVGIcon } from './SVGIcon';

import type { SVGIconProps } from './common';

export function LeafIcon({ size = 32, fill }: SVGIconProps) {
  return (
    <SVGIcon fill={fill} size={size} viewBox="0 0 32 32">
      <path d="M31.604 4.203C28.143 1.58 22.817.014 17.357.014 10.603.014 5.1 2.372 2.258 6.483.923 8.414.185 10.7.064 13.279c-.108 2.296.278 4.835 1.146 7.567C4.175 11.959 12.454 4.999 22 4.999c0 0-8.932 2.351-14.548 9.631-.003.004-.078.097-.207.272a21.055 21.055 0 0 0-2.846 5.166 30.771 30.771 0 0 0-2.4 11.931h4s-.607-3.819.449-8.212c1.747.236 3.308.353 4.714.353 3.677 0 6.293-.796 8.231-2.504 1.736-1.531 2.694-3.587 3.707-5.764 1.548-3.325 3.302-7.094 8.395-10.005a1 1 0 0 0 .108-1.666z" />
    </SVGIcon>
  );
}
