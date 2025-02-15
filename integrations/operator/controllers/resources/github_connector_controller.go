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

package resources

import (
	"context"

	"github.com/gravitational/trace"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	resourcesv3 "github.com/gravitational/teleport/integrations/operator/apis/resources/v3"
)

// githubConnectorClient implements TeleportResourceClient and offers CRUD methods needed to reconcile github_connectors
type githubConnectorClient struct {
	teleportClient *client.Client
}

// Get gets the Teleport github_connector of a given name
func (r githubConnectorClient) Get(ctx context.Context, name string) (types.GithubConnector, error) {
	github, err := r.teleportClient.GetGithubConnector(ctx, name, false /* with secrets*/)
	return github, trace.Wrap(err)
}

// Create creates a Teleport github_connector
func (r githubConnectorClient) Create(ctx context.Context, github types.GithubConnector) error {
	_, err := r.teleportClient.CreateGithubConnector(ctx, github)
	return trace.Wrap(err)
}

// Update updates a Teleport github_connector
func (r githubConnectorClient) Update(ctx context.Context, github types.GithubConnector) error {
	_, err := r.teleportClient.UpsertGithubConnector(ctx, github)
	return trace.Wrap(err)
}

// Delete deletes a Teleport github_connector
func (r githubConnectorClient) Delete(ctx context.Context, name string) error {
	return trace.Wrap(r.teleportClient.DeleteGithubConnector(ctx, name))
}

// NewGithubConnectorReconciler instantiates a new Kubernetes controller reconciling github_connector resources
func NewGithubConnectorReconciler(client kclient.Client, tClient *client.Client) *TeleportResourceReconciler[types.GithubConnector, *resourcesv3.TeleportGithubConnector] {
	githubClient := &githubConnectorClient{
		teleportClient: tClient,
	}

	resourceReconciler := NewTeleportResourceReconciler[types.GithubConnector, *resourcesv3.TeleportGithubConnector](
		client,
		githubClient,
	)

	return resourceReconciler
}
