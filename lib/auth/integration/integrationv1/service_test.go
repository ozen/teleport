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

package integrationv1

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/teleport/api/client/proto"
	integrationpb "github.com/gravitational/teleport/api/gen/proto/go/teleport/integration/v1"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/auth/keystore"
	"github.com/gravitational/teleport/lib/auth/testauthority"
	"github.com/gravitational/teleport/lib/authz"
	"github.com/gravitational/teleport/lib/backend/memory"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/services/local"
	"github.com/gravitational/teleport/lib/tlsca"
)

func TestIntegrationCRUD(t *testing.T) {
	t.Parallel()
	clusterName := "test-cluster"

	ca := newCertAuthority(t, types.HostCA, clusterName)
	ctx, localClient, resourceSvc := initSvc(t, types.KindIntegration, ca, clusterName)

	noError := func(err error) bool {
		return err == nil
	}

	sampleIntegrationFn := func(t *testing.T, name string) types.Integration {
		ig, err := types.NewIntegrationAWSOIDC(
			types.Metadata{Name: name},
			&types.AWSOIDCIntegrationSpecV1{
				RoleARN: "arn:aws:iam::123456789012:role/OpsTeam",
			},
		)
		require.NoError(t, err)
		return ig
	}

	tt := []struct {
		Name         string
		Role         types.RoleSpecV6
		Setup        func(t *testing.T, igName string)
		Test         func(ctx context.Context, resourceSvc *Service, igName string) error
		ErrAssertion func(error) bool
	}{
		// Read
		{
			Name: "allowed read access to integrations",
			Role: types.RoleSpecV6{
				Allow: types.RoleConditions{Rules: []types.Rule{{
					Resources: []string{types.KindIntegration},
					Verbs:     []string{types.VerbRead},
				}}},
			},
			Setup: func(t *testing.T, igName string) {
				_, err := localClient.CreateIntegration(ctx, sampleIntegrationFn(t, igName))
				require.NoError(t, err)
			},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.GetIntegration(ctx, &integrationpb.GetIntegrationRequest{
					Name: igName,
				})
				return err
			},
			ErrAssertion: noError,
		},
		{
			Name: "no access to read integrations",
			Role: types.RoleSpecV6{},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.GetIntegration(ctx, &integrationpb.GetIntegrationRequest{
					Name: igName,
				})
				return err
			},
			ErrAssertion: trace.IsAccessDenied,
		},
		{
			Name: "denied access to read integrations",
			Role: types.RoleSpecV6{
				Deny: types.RoleConditions{Rules: []types.Rule{{
					Resources: []string{types.KindIntegration},
					Verbs:     []string{types.VerbRead},
				}}},
			},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.GetIntegration(ctx, &integrationpb.GetIntegrationRequest{
					Name: igName,
				})
				return err
			},
			ErrAssertion: trace.IsAccessDenied,
		},

		// List
		{
			Name: "allowed list access to integrations",
			Role: types.RoleSpecV6{
				Allow: types.RoleConditions{Rules: []types.Rule{{
					Resources: []string{types.KindIntegration},
					Verbs:     []string{types.VerbList, types.VerbRead},
				}}},
			},
			Setup: func(t *testing.T, _ string) {
				for i := 0; i < 10; i++ {
					_, err := localClient.CreateIntegration(ctx, sampleIntegrationFn(t, uuid.NewString()))
					require.NoError(t, err)
				}
			},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.ListIntegrations(ctx, &integrationpb.ListIntegrationsRequest{
					Limit:   0,
					NextKey: "",
				})
				return err
			},
			ErrAssertion: noError,
		},
		{
			Name: "no list access to integrations",
			Role: types.RoleSpecV6{
				Allow: types.RoleConditions{Rules: []types.Rule{{
					Resources: []string{types.KindIntegration},
					Verbs:     []string{types.VerbCreate},
				}}},
			},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.ListIntegrations(ctx, &integrationpb.ListIntegrationsRequest{
					Limit:   0,
					NextKey: "",
				})
				return err
			},
			ErrAssertion: trace.IsAccessDenied,
		},

		// Create
		{
			Name: "no access to create integrations",
			Role: types.RoleSpecV6{},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				ig := sampleIntegrationFn(t, igName)
				_, err := resourceSvc.CreateIntegration(ctx, &integrationpb.CreateIntegrationRequest{Integration: ig.(*types.IntegrationV1)})
				return err
			},
			ErrAssertion: trace.IsAccessDenied,
		},
		{
			Name: "access to create integrations",
			Role: types.RoleSpecV6{
				Allow: types.RoleConditions{Rules: []types.Rule{{
					Resources: []string{types.KindIntegration},
					Verbs:     []string{types.VerbCreate},
				}}},
			},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				ig := sampleIntegrationFn(t, igName)
				_, err := resourceSvc.CreateIntegration(ctx, &integrationpb.CreateIntegrationRequest{Integration: ig.(*types.IntegrationV1)})
				return err
			},
			ErrAssertion: noError,
		},

		// Update
		{
			Name: "no access to update integration",
			Role: types.RoleSpecV6{},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				ig := sampleIntegrationFn(t, igName)
				_, err := resourceSvc.UpdateIntegration(ctx, &integrationpb.UpdateIntegrationRequest{Integration: ig.(*types.IntegrationV1)})
				return err
			},
			ErrAssertion: trace.IsAccessDenied,
		},
		{
			Name: "access to update integration",
			Role: types.RoleSpecV6{
				Allow: types.RoleConditions{Rules: []types.Rule{{
					Resources: []string{types.KindIntegration},
					Verbs:     []string{types.VerbUpdate},
				}}},
			},
			Setup: func(t *testing.T, igName string) {
				_, err := localClient.CreateIntegration(ctx, sampleIntegrationFn(t, igName))
				require.NoError(t, err)
			},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				ig := sampleIntegrationFn(t, igName)
				_, err := resourceSvc.UpdateIntegration(ctx, &integrationpb.UpdateIntegrationRequest{Integration: ig.(*types.IntegrationV1)})
				return err
			},
			ErrAssertion: noError,
		},

		// Delete
		{
			Name: "no access to delete integration",
			Role: types.RoleSpecV6{},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.DeleteIntegration(ctx, &integrationpb.DeleteIntegrationRequest{Name: "x"})
				return err
			},
			ErrAssertion: trace.IsAccessDenied,
		},
		{
			Name: "access to delete integration",
			Role: types.RoleSpecV6{
				Allow: types.RoleConditions{Rules: []types.Rule{{
					Resources: []string{types.KindIntegration},
					Verbs:     []string{types.VerbDelete},
				}}},
			},
			Setup: func(t *testing.T, igName string) {
				_, err := localClient.CreateIntegration(ctx, sampleIntegrationFn(t, igName))
				require.NoError(t, err)
			},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.DeleteIntegration(ctx, &integrationpb.DeleteIntegrationRequest{Name: igName})
				return err
			},
			ErrAssertion: noError,
		},

		// Delete all
		{
			Name: "remove all integrations fails when no access",
			Role: types.RoleSpecV6{},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.DeleteAllIntegrations(ctx, &integrationpb.DeleteAllIntegrationsRequest{})
				return err
			},
			ErrAssertion: trace.IsAccessDenied,
		},
		{
			Name: "remove all integrations",
			Role: types.RoleSpecV6{
				Allow: types.RoleConditions{Rules: []types.Rule{{
					Resources: []string{types.KindIntegration},
					Verbs:     []string{types.VerbDelete},
				}}},
			},
			Setup: func(t *testing.T, _ string) {
				for i := 0; i < 10; i++ {
					_, err := localClient.CreateIntegration(ctx, sampleIntegrationFn(t, uuid.NewString()))
					require.NoError(t, err)
				}
			},
			Test: func(ctx context.Context, resourceSvc *Service, igName string) error {
				_, err := resourceSvc.DeleteAllIntegrations(ctx, &integrationpb.DeleteAllIntegrationsRequest{})
				return err
			},
			ErrAssertion: noError,
		},
	}

	for _, tc := range tt {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			localCtx := authorizerForDummyUser(t, ctx, tc.Role, localClient)

			igName := uuid.NewString()
			if tc.Setup != nil {
				tc.Setup(t, igName)
			}

			err := tc.Test(localCtx, resourceSvc, igName)
			require.True(t, tc.ErrAssertion(err), err)
		})
	}
}

func authorizerForDummyUser(t *testing.T, ctx context.Context, roleSpec types.RoleSpecV6, localClient localClient) context.Context {
	// Create role
	roleName := "role-" + uuid.NewString()
	role, err := types.NewRole(roleName, roleSpec)
	require.NoError(t, err)

	role, err = localClient.CreateRole(ctx, role)
	require.NoError(t, err)

	// Create user
	user, err := types.NewUser("user-" + uuid.NewString())
	require.NoError(t, err)
	user.AddRole(roleName)
	user, err = localClient.CreateUser(ctx, user)
	require.NoError(t, err)

	return authz.ContextWithUser(ctx, authz.LocalUser{
		Username: user.GetName(),
		Identity: tlsca.Identity{
			Username: user.GetName(),
			Groups:   []string{role.GetName()},
		},
	})
}

type localClient interface {
	CreateUser(ctx context.Context, user types.User) (types.User, error)
	CreateRole(ctx context.Context, role types.Role) (types.Role, error)
	CreateIntegration(ctx context.Context, ig types.Integration) (types.Integration, error)
}

type testClient struct {
	services.ClusterConfiguration
	services.Trust
	services.RoleGetter
	services.UserGetter
}

func (c *testClient) ValidateMFAAuthResponse(ctx context.Context, resp *proto.MFAAuthenticateResponse, user string, passwordless bool) (*types.MFADevice, string, error) {
	return nil, "", nil
}

func initSvc(t *testing.T, kind string, ca types.CertAuthority, clusterName string) (context.Context, localClient, *Service) {
	ctx := context.Background()
	backend, err := memory.New(memory.Config{})
	require.NoError(t, err)

	clusterConfigSvc, err := local.NewClusterConfigurationService(backend)
	require.NoError(t, err)
	trustSvc := local.NewCAService(backend)
	roleSvc := local.NewAccessService(backend)
	userSvc := local.NewIdentityService(backend)

	require.NoError(t, clusterConfigSvc.SetAuthPreference(ctx, types.DefaultAuthPreference()))
	require.NoError(t, clusterConfigSvc.SetClusterAuditConfig(ctx, types.DefaultClusterAuditConfig()))
	require.NoError(t, clusterConfigSvc.SetClusterNetworkingConfig(ctx, types.DefaultClusterNetworkingConfig()))
	require.NoError(t, clusterConfigSvc.SetSessionRecordingConfig(ctx, types.DefaultSessionRecordingConfig()))

	accessPoint := &testClient{
		ClusterConfiguration: clusterConfigSvc,
		Trust:                trustSvc,
		RoleGetter:           roleSvc,
		UserGetter:           userSvc,
	}

	accessService := local.NewAccessService(backend)
	eventService := local.NewEventsService(backend)
	lockWatcher, err := services.NewLockWatcher(ctx, services.LockWatcherConfig{
		ResourceWatcherConfig: services.ResourceWatcherConfig{
			Client:    eventService,
			Component: "test",
		},
		LockGetter: accessService,
	})
	require.NoError(t, err)

	authorizer, err := authz.NewAuthorizer(authz.AuthorizerOpts{
		ClusterName: clusterName,
		AccessPoint: accessPoint,
		LockWatcher: lockWatcher,
	})
	require.NoError(t, err)

	localResourceService, err := local.NewIntegrationsService(backend)
	require.NoError(t, err)

	keystoreManager, err := keystore.NewManager(ctx, keystore.Config{
		Software: keystore.SoftwareConfig{
			RSAKeyPairSource: testauthority.New().GenerateKeyPair,
		},
	})
	require.NoError(t, err)

	caGetter := &mockCAGetter{
		domainName: clusterName,
		ca:         ca,
		keystore:   keystoreManager,
	}

	resourceSvc, err := NewService(&ServiceConfig{
		Backend:    localResourceService,
		Authorizer: authorizer,
		Cache:      localResourceService,
		CAGetter:   caGetter,
	})
	require.NoError(t, err)

	return ctx, struct {
		*local.AccessService
		*local.IdentityService
		*local.IntegrationsService
	}{
		AccessService:       roleSvc,
		IdentityService:     userSvc,
		IntegrationsService: localResourceService,
	}, resourceSvc
}

// mockCAGetter implements CAGetter.
type mockCAGetter struct {
	domainName string
	ca         types.CertAuthority
	keystore   *keystore.Manager
}

// GetDomainName returns local auth domain of the current auth server
func (m *mockCAGetter) GetDomainName() (string, error) {
	return m.domainName, nil
}

// GetCertAuthority returns certificate authority by given id. Parameter loadSigningKeys
// controls if signing keys are loaded
func (m *mockCAGetter) GetCertAuthority(ctx context.Context, id types.CertAuthID, loadSigningKeys bool) (types.CertAuthority, error) {
	return m.ca, nil
}

// GetKeyStore returns the KeyStore used by the auth server
func (m *mockCAGetter) GetKeyStore() *keystore.Manager {
	return m.keystore
}

func newCertAuthority(t *testing.T, caType types.CertAuthType, domain string) types.CertAuthority {
	t.Helper()

	ta := testauthority.New()
	pub, priv, err := ta.GenerateJWT()
	require.NoError(t, err)

	ca, err := types.NewCertAuthority(types.CertAuthoritySpecV2{
		Type:        caType,
		ClusterName: domain,
		ActiveKeys: types.CAKeySet{
			JWT: []*types.JWTKeyPair{{
				PublicKey:      pub,
				PrivateKey:     priv,
				PrivateKeyType: types.PrivateKeyType_RAW,
			}},
		},
	})
	require.NoError(t, err)

	return ca
}
