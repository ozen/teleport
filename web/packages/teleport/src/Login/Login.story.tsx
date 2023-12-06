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

import LoginSuccess from './LoginSuccess';
import { LoginFailed } from './LoginFailed';
import { Login } from './Login';
import { State } from './useLogin';

export default {
  title: 'Teleport/Login',
};

export const MfaOff = () => <Login {...sample} />;
export const Otp = () => <Login {...sample} auth2faType="otp" />;
export const Webauthn = () => <Login {...sample} auth2faType="webauthn" />;
export const Optional = () => <Login {...sample} auth2faType="optional" />;
export const On = () => <Login {...sample} auth2faType="on" />;
export const Success = () => <LoginSuccess />;
export const FailedDefault = () => <LoginFailed />;
export const FailedCustom = () => <LoginFailed message="custom message" />;

const sample: State = {
  attempt: {
    isProcessing: false,
    isFailed: false,
    isSuccess: true,
    message: '',
  },
  onLogin: () => null,
  onLoginWithWebauthn: () => null,
  onLoginWithSso: () => null,
  authProviders: [],
  auth2faType: 'off',
  preferredMfaType: 'webauthn',
  isLocalAuthEnabled: true,
  clearAttempt: () => null,
  isPasswordlessEnabled: false,
  primaryAuthType: 'local',
  motd: '',
  showMotd: false,
  acknowledgeMotd: () => null,
};
