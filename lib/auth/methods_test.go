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

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apievents "github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/teleport/lib/events/eventstest"
)

func TestServerAuthenticateUserUserAgentTrim(t *testing.T) {
	ctx := context.Background()
	emitter := &eventstest.MockRecorderEmitter{}
	r := AuthenticateUserRequest{
		ClientMetadata: &ForwardedClientMetadata{
			UserAgent: strings.Repeat("A", maxUserAgentLen+1),
		},
	}
	// Ignoring the error here because we really just care that the event was logged.
	(&Server{emitter: emitter}).AuthenticateUser(ctx, r)
	event := emitter.LastEvent()
	loginEvent, ok := event.(*apievents.UserLogin)
	require.True(t, ok)
	require.LessOrEqual(t, len(loginEvent.UserAgent), maxUserAgentLen)
}
