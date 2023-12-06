/*
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

package services

import (
	"github.com/gravitational/trace"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/utils"
)

// UnmarshalInstaller unmarshals the installer resource from JSON.
func UnmarshalInstaller(data []byte, opts ...MarshalOption) (types.Installer, error) {
	var installer types.InstallerV1

	if len(data) == 0 {
		return nil, trace.BadParameter("missing resource data")
	}

	cfg, err := CollectOptions(opts)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if err := utils.FastUnmarshal(data, &installer); err != nil {
		return nil, trace.BadParameter(err.Error())
	}
	if err := installer.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	if cfg.ID != 0 {
		installer.SetResourceID(cfg.ID)
	}
	if cfg.Revision != "" {
		installer.SetRevision(cfg.Revision)
	}
	if !cfg.Expires.IsZero() {
		installer.SetExpiry(cfg.Expires)
	}
	return &installer, nil
}

// MarshalInstaller marshals the Installer resource to JSON.
func MarshalInstaller(installer types.Installer, opts ...MarshalOption) ([]byte, error) {
	if err := installer.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	cfg, err := CollectOptions(opts)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	switch installer := installer.(type) {
	case *types.InstallerV1:
		if !cfg.PreserveResourceID {
			// avoid modifying the original object
			// to prevent unexpected data races
			copy := *installer
			copy.SetResourceID(0)
			copy.SetRevision("")
			installer = &copy
		}
		return utils.FastMarshal(installer)
	default:
		return nil, trace.BadParameter("unrecognized installer version %T", installer)
	}
}
