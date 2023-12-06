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

import cfg from 'teleport/config';
import api from 'teleport/services/api';
import auth, { makeWebauthnCreationResponse } from 'teleport/services/auth';

import {
  MfaDevice,
  AddNewTotpDeviceRequest,
  AddNewHardwareDeviceRequest,
} from './types';
import makeMfaDevice from './makeMfaDevice';

class MfaService {
  fetchDevicesWithToken(tokenId: string): Promise<MfaDevice[]> {
    return api
      .get(cfg.getMfaDevicesWithTokenUrl(tokenId))
      .then(devices => devices.map(makeMfaDevice));
  }

  removeDevice(tokenId: string, deviceName: string) {
    return api.delete(cfg.getMfaDeviceUrl(tokenId, deviceName));
  }

  fetchDevices(): Promise<MfaDevice[]> {
    return api
      .get(cfg.api.mfaDevicesPath)
      .then(devices => devices.map(makeMfaDevice));
  }

  addNewTotpDevice(req: AddNewTotpDeviceRequest) {
    return api.post(cfg.api.mfaDevicesPath, req);
  }

  addNewWebauthnDevice(req: AddNewHardwareDeviceRequest) {
    return auth
      .checkWebauthnSupport()
      .then(() =>
        auth.createMfaRegistrationChallenge(
          req.tokenId,
          'webauthn',
          req.deviceUsage
        )
      )
      .then(res =>
        navigator.credentials.create({
          publicKey: res.webauthnPublicKey,
        })
      )
      .then(res => {
        const request = {
          ...req,
          webauthnRegisterResponse: makeWebauthnCreationResponse(res),
        };

        return api.post(cfg.api.mfaDevicesPath, request);
      });
  }
}

export default MfaService;
