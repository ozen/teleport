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

package main

import (
	"path/filepath"

	"github.com/gravitational/trace"

	"github.com/gravitational/teleport/lib/tbot/config"
	"github.com/gravitational/teleport/lib/tbot/tshwrap"
)

func onProxyCommand(botConfig *config.BotConfig, cf *config.CLIConf) error {
	wrapper, err := tshwrap.New()
	if err != nil {
		return trace.Wrap(err)
	}

	if err := tshwrap.CheckTSHSupported(wrapper); err != nil {
		return trace.Wrap(err)
	}

	destination, err := tshwrap.GetDestinationDirectory(botConfig)
	if err != nil {
		return trace.Wrap(err)
	}

	env, err := tshwrap.GetEnvForTSH(destination.Path)
	if err != nil {
		return trace.Wrap(err)
	}

	identityPath := filepath.Join(destination.Path, config.IdentityFilePath)
	if err != nil {
		return trace.Wrap(err)
	}

	// TODO(timothyb89):  We could consider supporting a --cluster passthrough
	//  here as in `tbot db ...`.
	args := []string{"-i", identityPath, "proxy", "--proxy=" + cf.Proxy}
	args = append(args, cf.RemainingArgs...)

	// Pass through the debug flag, and prepend to satisfy argument ordering
	// needs (`-d` must precede `proxy`).
	if botConfig.Debug {
		args = append([]string{"-d"}, args...)
	}

	return trace.Wrap(wrapper.Exec(env, args...), "executing `tsh proxy`")
}
