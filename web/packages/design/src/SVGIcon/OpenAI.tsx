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

import { SVGIcon } from 'design/SVGIcon/SVGIcon';

import type { SVGIconProps } from './common';

export function OpenAIIcon({ size = 20, fill }: SVGIconProps) {
  return (
    <SVGIcon fill={fill} size={size} viewBox="0 0 671.2 680.25">
      <path d="M626.95 278.4a169.444 169.444 0 0 0-14.56-139.19C575.28 74.58 500.66 41.34 427.8 56.98A169.52 169.52 0 0 0 299.98 0c-74.5-.18-140.58 47.78-163.5 118.67a169.546 169.546 0 0 0-113.32 82.21c-37.4 64.45-28.88 145.68 21.08 200.97A169.444 169.444 0 0 0 58.8 541.04c37.11 64.63 111.73 97.87 184.6 82.23a169.448 169.448 0 0 0 127.81 56.98c74.54.19 140.65-47.81 163.55-118.74 47.85-9.79 89.15-39.76 113.32-82.21 37.35-64.45 28.81-145.64-21.14-200.9ZM371.27 635.77c-29.82.04-58.7-10.4-81.6-29.5 1.03-.56 2.84-1.56 4.02-2.28l135.44-78.24a22.024 22.024 0 0 0 11.13-19.27V315.53l57.25 33.06c.61.3 1.03.89 1.11 1.57V508.3c-.09 70.32-57.04 127.32-127.36 127.48ZM97.37 518.79a127.061 127.061 0 0 1-15.21-85.43c1.01.6 2.76 1.68 4.02 2.4L221.62 514a22.032 22.032 0 0 0 22.25 0l165.36-95.48v66.11c.04.69-.27 1.34-.82 1.76l-136.92 79.05c-60.98 35.12-138.88 14.25-174.13-46.65ZM61.74 223.11a126.968 126.968 0 0 1 66.36-55.89c0 1.17-.07 3.23-.07 4.67v156.47c-.05 7.96 4.2 15.32 11.12 19.26l165.36 95.47-57.25 33.05c-.57.38-1.3.44-1.93.18L108.4 397.2c-60.87-35.25-81.74-113.11-46.65-174.08ZM532.1 332.57l-165.36-95.48 57.25-33.04c.57-.38 1.3-.44 1.93-.17l136.93 79.05c60.98 35.22 81.86 113.21 46.64 174.18a127.475 127.475 0 0 1-66.34 55.87V351.83a22.01 22.01 0 0 0-11.05-19.26Zm56.98-85.76c-1.33-.82-2.67-1.62-4.02-2.4l-135.45-78.24a22.08 22.08 0 0 0-22.25 0L262 261.65v-66.11c-.04-.69.27-1.34.82-1.76l136.92-78.99c61-35.17 138.96-14.24 174.13 46.76a127.506 127.506 0 0 1 15.21 85.25Zm-358.2 117.84-57.26-33.06c-.61-.3-1.03-.89-1.11-1.57V171.88c.05-70.41 57.17-127.46 127.58-127.41 29.78.02 58.61 10.46 81.49 29.51-1.03.56-2.83 1.56-4.02 2.28L242.12 154.5a21.996 21.996 0 0 0-11.13 19.26l-.09 190.89Zm31.1-67.05 73.65-42.54 73.65 42.51v85.05l-73.65 42.51-73.65-42.51V297.6Z" />
    </SVGIcon>
  );
}
