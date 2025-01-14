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

import React, { useState } from 'react';

import Text from '../Text';
import Flex from '../Flex';

import { Toggle } from './Toggle';

export default {
  title: 'Design/Toggle',
};

export const Default = () => {
  const [toggled, setToggled] = useState(false);

  function toggle(): void {
    setToggled(wasToggled => !wasToggled);
  }

  return (
    <Flex gap={3}>
      <div>
        <Text>Enabled</Text>
        <Toggle onToggle={toggle} isToggled={toggled} />
      </div>
      <div>
        <Text>Disabled</Text>
        <Toggle disabled={true} onToggle={toggle} isToggled={toggled} />
      </div>
    </Flex>
  );
};
