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

package usagereporter

import (
	"github.com/gravitational/teleport/api/types"
	apievents "github.com/gravitational/teleport/api/types/events"
	prehogv1a "github.com/gravitational/teleport/gen/proto/go/prehog/v1alpha"
	"github.com/gravitational/teleport/lib/events"
)

const (
	// TCPSessionType is the session_type in tp.session.start for TCP
	// Application Access.
	TCPSessionType = "app_tcp"
	// PortSessionType is the session_type in tp.session.start for SSH or Kube
	// port forwarding.
	//
	// Deprecated: used in older versions to mean either SSH or Kube. Use
	// PortSSHSessionType or PortKubeSessionType instead.
	PortSessionType = "ssh_port"
	// PortSSHSessionType is the session_type in tp.session.start for SSH port
	// forwarding.
	PortSSHSessionType = "ssh_port_v2"
	// PortKubeSessionType is the session_type in tp.session.start for Kube port
	// forwarding.
	PortKubeSessionType = "k8s_port"
)

func ConvertAuditEvent(event apievents.AuditEvent) Anonymizable {
	switch e := event.(type) {
	case *apievents.UserLogin:
		// Only count successful logins.
		if !e.Success {
			return nil
		}

		var deviceID string
		if e.TrustedDevice != nil {
			deviceID = e.TrustedDevice.DeviceId
		}

		// Note: we can have different behavior based on event code (local vs
		// SSO) if desired, but we currently only care about connector type /
		// method
		return &UserLoginEvent{
			UserName:                 e.User,
			ConnectorType:            e.Method,
			DeviceId:                 deviceID,
			RequiredPrivateKeyPolicy: e.RequiredPrivateKeyPolicy,
		}

	case *apievents.SessionStart:
		// Note: session.start is only SSH and Kubernetes.
		sessionType := types.SSHSessionKind
		if e.KubernetesCluster != "" {
			sessionType = types.KubernetesSessionKind
		}

		return &SessionStartEvent{
			UserName:    e.User,
			SessionType: string(sessionType),
		}
	case *apievents.PortForward:
		sessionType := PortSSHSessionType
		if e.ConnectionMetadata.Protocol == events.EventProtocolKube {
			sessionType = PortKubeSessionType
		}
		return &SessionStartEvent{
			UserName:    e.User,
			SessionType: sessionType,
		}
	case *apievents.DatabaseSessionStart:
		return &SessionStartEvent{
			UserName:    e.User,
			SessionType: string(types.DatabaseSessionKind),
			Database: &prehogv1a.SessionStartDatabaseMetadata{
				DbType:     e.DatabaseType,
				DbProtocol: e.DatabaseProtocol,
				DbOrigin:   e.DatabaseOrigin,
			},
		}
	case *apievents.AppSessionStart:
		sessionType := string(types.AppSessionKind)
		if types.IsAppTCP(e.AppURI) {
			sessionType = TCPSessionType
		}
		return &SessionStartEvent{
			UserName:    e.User,
			SessionType: sessionType,
		}
	case *apievents.WindowsDesktopSessionStart:
		desktopType := "ad"
		if e.DesktopLabels[types.ADLabel] == "false" {
			desktopType = "non-ad"
		}
		return &SessionStartEvent{
			UserName:    e.User,
			SessionType: string(types.WindowsDesktopSessionKind),
			Desktop: &prehogv1a.SessionStartDesktopMetadata{
				DesktopType:       desktopType,
				Origin:            e.DesktopLabels[types.OriginLabel],
				WindowsDomain:     e.Domain,
				AllowUserCreation: e.AllowUserCreation,
			},
		}

	case *apievents.GithubConnectorCreate:
		return &SSOCreateEvent{
			ConnectorType: types.KindGithubConnector,
		}
	case *apievents.OIDCConnectorCreate:
		return &SSOCreateEvent{
			ConnectorType: types.KindOIDCConnector,
		}
	case *apievents.SAMLConnectorCreate:
		return &SSOCreateEvent{
			ConnectorType: types.KindSAMLConnector,
		}

	case *apievents.RoleCreate:
		return &RoleCreateEvent{
			UserName: e.User,
			RoleName: e.ResourceMetadata.Name,
		}

	case *apievents.KubeRequest:
		return &KubeRequestEvent{
			UserName: e.User,
		}

	case *apievents.SFTP:
		return &SFTPEvent{
			UserName: e.User,
			Action:   int32(e.Action),
		}

	case *apievents.BotJoin:
		// Only count successful joins.
		if !e.Success {
			return nil
		}
		return &BotJoinEvent{
			BotName:       e.BotName,
			JoinMethod:    e.Method,
			JoinTokenName: e.TokenName,
		}

	case *apievents.DeviceEvent2:
		// Only count successful events.
		if !e.Success {
			return nil
		}

		switch e.Metadata.GetType() {
		case events.DeviceAuthenticateEvent:
			return &DeviceAuthenticateEvent{
				DeviceId:     e.Device.DeviceId,
				UserName:     e.User,
				DeviceOsType: e.Device.OsType.String(),
			}
		case events.DeviceEnrollEvent:
			return &DeviceEnrollEvent{
				DeviceId:     e.Device.DeviceId,
				UserName:     e.User,
				DeviceOsType: e.Device.OsType.String(),
				DeviceOrigin: e.Device.DeviceOrigin.String(),
			}
		}

	case *apievents.DesktopClipboardReceive:
		return &DesktopClipboardEvent{
			Desktop:  e.DesktopAddr,
			UserName: e.User,
		}
	case *apievents.DesktopClipboardSend:
		return &DesktopClipboardEvent{
			Desktop:  e.DesktopAddr,
			UserName: e.User,
		}
	case *apievents.DesktopSharedDirectoryStart:
		// only count successful share attempts
		if e.Code != events.DesktopSharedDirectoryStartCode {
			return nil
		}

		return &DesktopDirectoryShareEvent{
			Desktop:       e.DesktopAddr,
			UserName:      e.User,
			DirectoryName: e.DirectoryName,
		}
	case *apievents.AuditQueryRun:
		return &AuditQueryRunEvent{
			UserName:  e.User,
			Days:      e.Days,
			IsSuccess: e.Status.Success,
		}
	}

	return nil
}
