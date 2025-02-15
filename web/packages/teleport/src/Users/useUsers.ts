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

import { ReactElement, useState, useEffect } from 'react';
import { useAttempt } from 'shared/hooks';

import { User } from 'teleport/services/user';
import useTeleport from 'teleport/useTeleport';

export default function useUsers({
  InviteCollaborators,
  EmailPasswordReset,
}: UsersContainerProps) {
  const ctx = useTeleport();
  const [attempt, attemptActions] = useAttempt({ isProcessing: true });
  const [users, setUsers] = useState([] as User[]);
  const [roles, setRoles] = useState([] as string[]);
  const [operation, setOperation] = useState({
    type: 'none',
  } as Operation);
  const [inviteCollaboratorsOpen, setInviteCollaboratorsOpen] =
    useState<boolean>(false);

  function onStartCreate() {
    const user = { name: '', roles: [], created: new Date() };
    setOperation({
      type: 'create',
      user,
    });
  }

  function onStartEdit(user: User) {
    setOperation({ type: 'edit', user });
  }

  function onStartDelete(user: User) {
    setOperation({ type: 'delete', user });
  }

  function onStartReset(user: User) {
    setOperation({ type: 'reset', user });
  }

  function onStartInviteCollaborators(user: User) {
    setOperation({ type: 'invite-collaborators', user });
    setInviteCollaboratorsOpen(true);
  }

  function onClose() {
    setOperation({ type: 'none' });
  }

  function onReset(name: string) {
    return ctx.userService.createResetPasswordToken(name, 'password');
  }

  function onDelete(name: string) {
    return ctx.userService.deleteUser(name).then(() => {
      const updatedUsers = users.filter(user => user.name !== name);
      setUsers(updatedUsers);
    });
  }

  function onUpdate(u: User) {
    return ctx.userService.updateUser(u).then(result => {
      setUsers([result, ...users.filter(i => i.name !== u.name)]);
    });
  }

  function onCreate(u: User) {
    return ctx.userService
      .createUser(u)
      .then(result => setUsers([result, ...users]))
      .then(() => ctx.userService.createResetPasswordToken(u.name, 'invite'));
  }

  function onInviteCollaboratorsClose(newUsers?: User[]) {
    if (newUsers && newUsers.length > 0) {
      setUsers([...newUsers, ...users]);
    }

    setInviteCollaboratorsOpen(false);
    setOperation({ type: 'none' });
  }

  function onEmailPasswordResetClose() {
    setOperation({ type: 'none' });
  }

  useEffect(() => {
    function fetchRoles() {
      if (ctx.getFeatureFlags().roles) {
        return ctx.resourceService
          .fetchRoles()
          .then(resources => resources.map(role => role.name));
      }

      return Promise.resolve([]);
    }

    attemptActions.do(() =>
      Promise.all([fetchRoles(), ctx.userService.fetchUsers()]).then(values => {
        setRoles(values[0]);
        setUsers(values[1]);
      })
    );
  }, []);

  return {
    attempt,
    users,
    roles,
    operation,
    onStartCreate,
    onStartDelete,
    onStartEdit,
    onStartReset,
    onStartInviteCollaborators,
    onClose,
    onDelete,
    onCreate,
    onUpdate,
    onReset,
    onInviteCollaboratorsClose,
    InviteCollaborators,
    inviteCollaboratorsOpen,
    onEmailPasswordResetClose,
    EmailPasswordReset,
  };
}

type Operation = {
  type:
    | 'create'
    | 'invite-collaborators'
    | 'edit'
    | 'delete'
    | 'reset'
    | 'none';
  user?: User;
};

export interface InviteCollaboratorsDialogProps {
  onClose: (users?: User[]) => void;
  open: boolean;
}

export interface EmailPasswordResetDialogProps {
  username: string;
  onClose: () => void;
}

export type UsersContainerProps = {
  InviteCollaborators?: (props: InviteCollaboratorsDialogProps) => ReactElement;
  EmailPasswordReset?: (props: EmailPasswordResetDialogProps) => ReactElement;
};

export type State = ReturnType<typeof useUsers>;
