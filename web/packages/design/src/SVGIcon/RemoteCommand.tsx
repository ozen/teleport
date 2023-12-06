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

export function RemoteCommandIcon({ size = 24, fill }: SVGIconProps) {
  return (
    <SVGIcon fill={fill} size={size} viewBox="0 0 24 24">
      <path d="M 5 3 C 3.9069372 3 3 3.9069372 3 5 L 3 19 C 3 20.093063 3.9069372 21 5 21 L 19 21 C 20.093063 21 21 20.093063 21 19 L 21 5 C 21 3.9069372 20.093063 3 19 3 L 5 3 z M 5 8 L 19 8 L 19 19 L 5 19 L 5 8 z M 7.9941406 11 C 7.8078906 11 7.6205156 11.070891 7.4785156 11.212891 C 7.1945156 11.496891 7.1945156 11.957188 7.4785156 12.242188 L 9.2363281 14 L 7.4785156 15.757812 C 7.1945156 16.041812 7.1945156 16.501156 7.4785156 16.785156 C 7.7625156 17.069156 8.2238125 17.069156 8.5078125 16.785156 L 10.779297 14.513672 C 11.063297 14.229672 11.063297 13.768375 10.779297 13.484375 L 8.5078125 11.212891 C 8.3658125 11.070891 8.1803906 11 7.9941406 11 z M 12.75 15.5 C 12.336 15.5 12 15.836 12 16.25 C 12 16.664 12.336 17 12.75 17 L 16.25 17 C 16.664 17 17 16.664 17 16.25 C 17 15.836 16.664 15.5 16.25 15.5 L 12.75 15.5 z" />
    </SVGIcon>
  );
}
