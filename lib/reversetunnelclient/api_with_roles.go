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

package reversetunnelclient

import (
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/utils"
)

// ClusterGetter is an interface that defines GetRemoteCluster method
type ClusterGetter interface {
	// GetRemoteCluster returns a remote cluster by name
	GetRemoteCluster(clusterName string) (types.RemoteCluster, error)
}

// NewTunnelWithRoles returns new authorizing tunnel
func NewTunnelWithRoles(tunnel Tunnel, localCluster string, accessChecker services.AccessChecker, access ClusterGetter) *TunnelWithRoles {
	return &TunnelWithRoles{
		tunnel:        tunnel,
		localCluster:  localCluster,
		accessChecker: accessChecker,
		access:        access,
	}
}

// TunnelWithRoles authorizes requests
type TunnelWithRoles struct {
	tunnel Tunnel

	localCluster string

	// accessChecker is used to check RBAC permissions.
	accessChecker services.AccessChecker

	access ClusterGetter
}

// GetSites returns a list of connected remote sites
func (t *TunnelWithRoles) GetSites() ([]RemoteSite, error) {
	clusters, err := t.tunnel.GetSites()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	out := make([]RemoteSite, 0, len(clusters))
	for _, cluster := range clusters {
		if t.localCluster == cluster.GetName() {
			out = append(out, cluster)
			continue
		}
		rc, err := t.access.GetRemoteCluster(cluster.GetName())
		if err != nil {
			if !trace.IsNotFound(err) {
				return nil, trace.Wrap(err)
			}
			logrus.Warningf("Skipping dangling cluster %q, no remote cluster resource found.", cluster.GetName())
			continue
		}
		if err := t.accessChecker.CheckAccessToRemoteCluster(rc); err != nil {
			if !trace.IsAccessDenied(err) {
				return nil, trace.Wrap(err)
			}
			continue
		}
		out = append(out, cluster)
	}
	return out, nil
}

// GetSite returns remote site this node belongs to
func (t *TunnelWithRoles) GetSite(clusterName string) (RemoteSite, error) {
	cluster, err := t.tunnel.GetSite(clusterName)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if t.localCluster == cluster.GetName() {
		return cluster, nil
	}
	rc, err := t.access.GetRemoteCluster(clusterName)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if err := t.accessChecker.CheckAccessToRemoteCluster(rc); err != nil {
		return nil, utils.OpaqueAccessDenied(err)
	}
	return cluster, nil
}
