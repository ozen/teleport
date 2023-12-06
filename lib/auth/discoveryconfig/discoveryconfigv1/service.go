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

package discoveryconfigv1

import (
	"context"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"

	discoveryconfigv1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/discoveryconfig/v1"
	"github.com/gravitational/teleport/api/types"
	conv "github.com/gravitational/teleport/api/types/discoveryconfig/convert/v1"
	"github.com/gravitational/teleport/lib/authz"
	"github.com/gravitational/teleport/lib/services"
)

// ServiceConfig holds configuration options for the DiscoveryConfig gRPC service.
type ServiceConfig struct {
	// Logger is the logger to use.
	Logger logrus.FieldLogger

	// Authorizer is the authorizer to use.
	Authorizer authz.Authorizer

	// Backend is the backend for storing DiscoveryConfigs.
	Backend services.DiscoveryConfigs

	// Clock is the clock.
	Clock clockwork.Clock
}

// CheckAndSetDefaults checks the ServiceConfig fields and returns an error if
// a required param is not provided.
// Authorizer, Cache and Backend are required params
func (s *ServiceConfig) CheckAndSetDefaults() error {
	if s.Authorizer == nil {
		return trace.BadParameter("authorizer is required")
	}
	if s.Backend == nil {
		return trace.BadParameter("backend is required")
	}

	if s.Logger == nil {
		s.Logger = logrus.New().WithField(trace.Component, "discoveryconfig_crud_service")
	}

	if s.Clock == nil {
		s.Clock = clockwork.NewRealClock()
	}

	return nil
}

// Service implements the teleport.DiscoveryConfig.v1.DiscoveryConfigService RPC service.
type Service struct {
	discoveryconfigv1.UnimplementedDiscoveryConfigServiceServer

	log        logrus.FieldLogger
	authorizer authz.Authorizer
	backend    services.DiscoveryConfigs
	clock      clockwork.Clock
}

// NewService returns a new DiscoveryConfigs gRPC service.
func NewService(cfg ServiceConfig) (*Service, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	return &Service{
		log:        cfg.Logger,
		authorizer: cfg.Authorizer,
		backend:    cfg.Backend,
		clock:      cfg.Clock,
	}, nil
}

// ListDiscoveryConfigs returns a paginated list of all DiscoveryConfig resources.
func (s *Service) ListDiscoveryConfigs(ctx context.Context, req *discoveryconfigv1.ListDiscoveryConfigsRequest) (*discoveryconfigv1.ListDiscoveryConfigsResponse, error) {
	_, err := authz.AuthorizeWithVerbs(ctx, s.log, s.authorizer, true, types.KindDiscoveryConfig, types.VerbRead, types.VerbList)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	results, nextKey, err := s.backend.ListDiscoveryConfigs(ctx, int(req.GetPageSize()), req.GetNextToken())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	dcs := make([]*discoveryconfigv1.DiscoveryConfig, len(results))
	for i, r := range results {
		dcs[i] = conv.ToProto(r)
	}

	return &discoveryconfigv1.ListDiscoveryConfigsResponse{
		DiscoveryConfigs: dcs,
		NextKey:          nextKey,
	}, nil
}

// GetDiscoveryConfig returns the specified DiscoveryConfig resource.
func (s *Service) GetDiscoveryConfig(ctx context.Context, req *discoveryconfigv1.GetDiscoveryConfigRequest) (*discoveryconfigv1.DiscoveryConfig, error) {
	_, err := authz.AuthorizeWithVerbs(ctx, s.log, s.authorizer, true, types.KindDiscoveryConfig, types.VerbRead)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	dc, err := s.backend.GetDiscoveryConfig(ctx, req.Name)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return conv.ToProto(dc), nil
}

// CreateDiscoveryConfig creates a new DiscoveryConfig resource.
func (s *Service) CreateDiscoveryConfig(ctx context.Context, req *discoveryconfigv1.CreateDiscoveryConfigRequest) (*discoveryconfigv1.DiscoveryConfig, error) {
	_, err := authz.AuthorizeWithVerbs(ctx, s.log, s.authorizer, true, types.KindDiscoveryConfig, types.VerbCreate)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	dc, err := conv.FromProto(req.GetDiscoveryConfig())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	resp, err := s.backend.CreateDiscoveryConfig(ctx, dc)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return conv.ToProto(resp), nil
}

// UpdateDiscoveryConfig updates an existing DiscoveryConfig.
func (s *Service) UpdateDiscoveryConfig(ctx context.Context, req *discoveryconfigv1.UpdateDiscoveryConfigRequest) (*discoveryconfigv1.DiscoveryConfig, error) {
	_, err := authz.AuthorizeWithVerbs(ctx, s.log, s.authorizer, true, types.KindDiscoveryConfig, types.VerbUpdate)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	dc, err := conv.FromProto(req.GetDiscoveryConfig())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	resp, err := s.backend.UpdateDiscoveryConfig(ctx, dc)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return conv.ToProto(resp), nil
}

// UpsertDiscoveryConfig creates or updates a DiscoveryConfig.
func (s *Service) UpsertDiscoveryConfig(ctx context.Context, req *discoveryconfigv1.UpsertDiscoveryConfigRequest) (*discoveryconfigv1.DiscoveryConfig, error) {
	_, err := authz.AuthorizeWithVerbs(ctx, s.log, s.authorizer, true, types.KindDiscoveryConfig, types.VerbCreate, types.VerbUpdate)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	dc, err := conv.FromProto(req.GetDiscoveryConfig())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	resp, err := s.backend.UpsertDiscoveryConfig(ctx, dc)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return conv.ToProto(resp), nil
}

// DeleteDiscoveryConfig removes the specified DiscoveryConfig resource.
func (s *Service) DeleteDiscoveryConfig(ctx context.Context, req *discoveryconfigv1.DeleteDiscoveryConfigRequest) (*emptypb.Empty, error) {
	_, err := authz.AuthorizeWithVerbs(ctx, s.log, s.authorizer, true, types.KindDiscoveryConfig, types.VerbDelete)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if err := s.backend.DeleteDiscoveryConfig(ctx, req.GetName()); err != nil {
		return nil, trace.Wrap(err)
	}

	return &emptypb.Empty{}, nil
}

// DeleteAllDiscoveryConfigs removes all DiscoveryConfig resources.
func (s *Service) DeleteAllDiscoveryConfigs(ctx context.Context, _ *discoveryconfigv1.DeleteAllDiscoveryConfigsRequest) (*emptypb.Empty, error) {
	_, err := authz.AuthorizeWithVerbs(ctx, s.log, s.authorizer, true, types.KindDiscoveryConfig, types.VerbDelete)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if err := s.backend.DeleteAllDiscoveryConfigs(ctx); err != nil {
		return nil, trace.Wrap(err)
	}

	return &emptypb.Empty{}, nil
}
