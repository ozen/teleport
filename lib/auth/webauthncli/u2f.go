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

package webauthncli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/flynn/u2f/u2fhid"
	"github.com/flynn/u2f/u2ftoken"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// DevicePollInterval is the interval between polling attempts on Webauthn or
// U2F devices.
// Used by otherwise tight loops such as RunOnU2FDevices and related methods,
// like Login.
var DevicePollInterval = 200 * time.Millisecond

// ErrAlreadyRegistered may be used by RunOnU2FDevices callbacks to signify that
// a certain authenticator is already registered, and thus should be removed
// from the loop.
var ErrAlreadyRegistered = errors.New("already registered")

var errKeyMissingOrNotVerified = errors.New("key missing or user presence not verified")

// Token represents the actions possible using an U2F/CTAP1 token.
type Token interface {
	CheckAuthenticate(req u2ftoken.AuthenticateRequest) error
	Authenticate(req u2ftoken.AuthenticateRequest) (*u2ftoken.AuthenticateResponse, error)
	Register(req u2ftoken.RegisterRequest) ([]byte, error)
}

// u2fDevices, u2fOpen and u2fNewToken allows tests to fake interactions with
// devices.
var u2fDevices = u2fhid.Devices
var u2fOpen = u2fhid.Open
var u2fNewToken = func(d u2ftoken.Device) Token {
	return u2ftoken.NewToken(d)
}

type deviceKey struct {
	Callback int
	Path     string
}

// RunOnU2FDevices polls for new U2F/CTAP1 devices and invokes the callbacks
// against them in regular intervals, running until either one callback succeeds
// or the context is canceled.
// Typically, each callback represents a {credential,rpid} pair to check against
// the device.
// Calling this method using a context without a cancel or deadline means it
// will execute until successful (which may be never).
// Most callers should prefer higher-abstraction functions such as Login.
func RunOnU2FDevices(ctx context.Context, runCredentials ...func(Token) error) error {
	ticker := time.NewTicker(DevicePollInterval)
	defer ticker.Stop()

	removedDevices := make(map[deviceKey]bool)
	for {
		switch err := runOnU2FDevicesOnce(removedDevices, runCredentials); {
		case errors.Is(err, errKeyMissingOrNotVerified):
			// This is expected to happen a few times.
		case err != nil:
			errMsg := err.Error()
			// suppress error spam, this error doesnt prevent u2f from working
			if !strings.Contains(errMsg, "hid: privilege violation") &&
				!strings.Contains(errMsg, "hid: not permitted") {
				log.WithError(err).Debug("Error interacting with U2F devices")
			}
		default: // OK, success.
			return nil
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		}
	}
}

func runOnU2FDevicesOnce(removedDevices map[deviceKey]bool, runCredentials []func(Token) error) error {
	// Ask for devices every iteration, the user may plug a new device.
	infos, err := u2fDevices()
	if err != nil {
		return trace.Wrap(err)
	}

	var swallowed []error
	for _, info := range infos {
		dev, err := u2fOpen(info)
		if err != nil {
			// u2fhid.Open is a bit more prone to errors, especially "hid: privilege
			// violation" errors. Try other devices before bailing.
			swallowed = append(swallowed, err)
			continue
		}

		token := u2fNewToken(dev)
		for i, fn := range runCredentials {
			key := deviceKey{Callback: i, Path: info.Path}
			if info.Path != "" && removedDevices[key] {
				// Device previously removed during loop (likely doesn't know the key
				// handle or is already registered).
				// We may get to a situation where all devices are removed, but we keep
				// on trying because the user may plug another device.
				continue
			}

			switch err := fn(token); {
			case err == nil:
				return nil // OK, we got it.
			case errors.Is(err, u2ftoken.ErrPresenceRequired):
				// Wait for user action, they will choose the device to use.
			case errors.Is(err, u2ftoken.ErrUnknownKeyHandle) || errors.Is(err, ErrAlreadyRegistered):
				removedDevices[key] = true // No need to try this anymore.
			case err != nil:
				swallowed = append(swallowed, err)
			}
		}
	}
	if len(swallowed) > 0 {
		return trace.NewAggregate(swallowed...)
	}

	return errKeyMissingOrNotVerified // don't wrap, simplifies comparisons
}
