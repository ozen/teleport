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

package discovery

import (
	"context"
	"sync"
	"time"

	"github.com/gravitational/trace"

	usageeventsv1 "github.com/gravitational/teleport/api/gen/proto/go/usageevents/v1"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/srv/discovery/common"
)

const appEventPrefix = "app/"

func (s *Server) startKubeAppsWatchers() error {
	if len(s.kubeAppsFetchers) == 0 {
		return nil
	}

	var (
		appResources types.ResourcesWithLabels
		mu           sync.Mutex
	)

	reconciler, err := services.NewReconciler(
		services.ReconcilerConfig{
			Matcher: func(_ types.ResourceWithLabels) bool { return true },
			GetCurrentResources: func() types.ResourcesWithLabelsMap {
				apps, err := s.AccessPoint.GetApps(s.ctx)
				if err != nil {
					s.Log.WithError(err).Warn("Unable to get applications from cache.")
					return nil
				}

				return types.Apps(filterResources(apps, types.OriginDiscoveryKubernetes, s.DiscoveryGroup)).AsResources().ToMap()
			},
			GetNewResources: func() types.ResourcesWithLabelsMap {
				mu.Lock()
				defer mu.Unlock()
				return appResources.ToMap()
			},
			Log:      s.Log.WithField("kind", types.KindApp),
			OnCreate: s.onAppCreate,
			OnUpdate: s.onAppUpdate,
			OnDelete: s.onAppDelete,
		},
	)
	if err != nil {
		return trace.Wrap(err)
	}

	watcher, err := common.NewWatcher(s.ctx, common.WatcherConfig{
		FetchersFn:     common.StaticFetchers(s.kubeAppsFetchers),
		Interval:       5 * time.Minute,
		Log:            s.Log.WithField("kind", types.KindApp),
		DiscoveryGroup: s.DiscoveryGroup,
		Origin:         types.OriginDiscoveryKubernetes,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	go watcher.Start()

	go func() {
		for {
			select {
			case newResources := <-watcher.ResourcesC():
				mu.Lock()
				appResources = newResources
				mu.Unlock()

				if err := reconciler.Reconcile(s.ctx); err != nil {
					s.Log.WithError(err).Warn("Unable to reconcile resources.")
				}

			case <-s.ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (s *Server) onAppCreate(ctx context.Context, rwl types.ResourceWithLabels) error {
	app, ok := rwl.(types.Application)
	if !ok {
		return trace.BadParameter("invalid type received; expected types.Application, received %T", app)
	}
	s.Log.Debugf("Creating app %s", app.GetName())
	err := s.AccessPoint.CreateApp(ctx, app)
	// If the resource already exists, it means that the resource was created
	// by a previous discovery_service instance that didn't support the discovery
	// group feature or the discovery group was changed.
	// In this case, we need to update the resource with the
	// discovery group label to ensure the user doesn't have to manually delete
	// the resource.
	if trace.IsAlreadyExists(err) {
		return trace.Wrap(s.onAppUpdate(ctx, rwl))
	}
	if err != nil {
		return trace.Wrap(err)
	}
	err = s.emitUsageEvents(map[string]*usageeventsv1.ResourceCreateEvent{
		appEventPrefix + app.GetName(): {
			ResourceType:   types.DiscoveredResourceApp,
			ResourceOrigin: types.OriginKubernetes,
			// CloudProvider is not set for apps created from Kubernetes services
		},
	})
	if err != nil {
		s.Log.WithError(err).Debug("Error emitting usage event.")
	}
	return nil
}

func (s *Server) onAppUpdate(ctx context.Context, rwl types.ResourceWithLabels) error {
	app, ok := rwl.(types.Application)
	if !ok {
		return trace.BadParameter("invalid type received; expected types.Application, received %T", app)
	}
	s.Log.Debugf("Updating app %s.", app.GetName())
	return trace.Wrap(s.AccessPoint.UpdateApp(ctx, app))
}

func (s *Server) onAppDelete(ctx context.Context, rwl types.ResourceWithLabels) error {
	app, ok := rwl.(types.Application)
	if !ok {
		return trace.BadParameter("invalid type received; expected types.Application, received %T", app)
	}
	s.Log.Debugf("Deleting app %s.", app.GetName())
	return trace.Wrap(s.AccessPoint.DeleteApp(ctx, app.GetName()))
}
