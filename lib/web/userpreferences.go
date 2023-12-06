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

package web

import (
	"net/http"

	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"

	userpreferencesv1 "github.com/gravitational/teleport/api/gen/proto/go/userpreferences/v1"
	"github.com/gravitational/teleport/lib/httplib"
	"github.com/gravitational/teleport/lib/reversetunnelclient"
)

// AssistUserPreferencesResponse is the JSON response for the assist user preferences.
type AssistUserPreferencesResponse struct {
	PreferredLogins []string                         `json:"preferredLogins"`
	ViewMode        userpreferencesv1.AssistViewMode `json:"viewMode"`
}

type preferencesMarketingParams struct {
	Campaign string `json:"campaign"`
	Source   string `json:"source"`
	Medium   string `json:"medium"`
	Intent   string `json:"intent"`
}

type OnboardUserPreferencesResponse struct {
	PreferredResources []userpreferencesv1.Resource `json:"preferredResources"`
	MarketingParams    preferencesMarketingParams   `json:"marketingParams"`
}

// ClusterUserPreferencesResponse is the JSON response for the user's cluster preferences.
type ClusterUserPreferencesResponse struct {
	PinnedResources []string `json:"pinnedResources"`
}

type UnifiedResourcePreferencesResponse struct {
	DefaultTab userpreferencesv1.DefaultTab `json:"defaultTab"`
	ViewMode   userpreferencesv1.ViewMode   `json:"viewMode"`
}

// UserPreferencesResponse is the JSON response for the user preferences.
type UserPreferencesResponse struct {
	Assist                     AssistUserPreferencesResponse      `json:"assist"`
	Theme                      userpreferencesv1.Theme            `json:"theme"`
	UnifiedResourcePreferences UnifiedResourcePreferencesResponse `json:"unifiedResourcePreferences"`
	Onboard                    OnboardUserPreferencesResponse     `json:"onboard"`
	ClusterPreferences         ClusterUserPreferencesResponse     `json:"clusterPreferences,omitempty"`
}

func (h *Handler) getUserClusterPreferences(_ http.ResponseWriter, r *http.Request, p httprouter.Params, sctx *SessionContext, site reversetunnelclient.RemoteSite) (interface{}, error) {
	authClient, err := sctx.GetUserClient(r.Context(), site)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	resp, err := authClient.GetUserPreferences(r.Context(), &userpreferencesv1.GetUserPreferencesRequest{})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return clusterPreferencesResponse(resp.Preferences.ClusterPreferences), nil
}

// updateUserClusterPreferences is a handler for PUT /webapi/user/preferences.
func (h *Handler) updateUserClusterPreferences(_ http.ResponseWriter, r *http.Request, p httprouter.Params, sctx *SessionContext, site reversetunnelclient.RemoteSite) (any, error) {
	req := UserPreferencesResponse{}

	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	authClient, err := sctx.GetUserClient(r.Context(), site)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	preferences := makePreferenceRequest(req)

	if err := authClient.UpsertUserPreferences(r.Context(), preferences); err != nil {
		return nil, trace.Wrap(err)
	}

	return OK(), nil
}

// getUserPreferences is a handler for GET /webapi/user/preferences.
func (h *Handler) getUserPreferences(_ http.ResponseWriter, r *http.Request, _ httprouter.Params, sctx *SessionContext) (any, error) {
	authClient, err := sctx.GetClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	resp, err := authClient.GetUserPreferences(r.Context(), &userpreferencesv1.GetUserPreferencesRequest{})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return userPreferencesResponse(resp.Preferences), nil
}

func makePreferenceRequest(req UserPreferencesResponse) *userpreferencesv1.UpsertUserPreferencesRequest {
	return &userpreferencesv1.UpsertUserPreferencesRequest{
		Preferences: &userpreferencesv1.UserPreferences{
			Theme: req.Theme,
			UnifiedResourcePreferences: &userpreferencesv1.UnifiedResourcePreferences{
				DefaultTab: req.UnifiedResourcePreferences.DefaultTab,
				ViewMode:   req.UnifiedResourcePreferences.ViewMode,
			},
			Assist: &userpreferencesv1.AssistUserPreferences{
				PreferredLogins: req.Assist.PreferredLogins,
				ViewMode:        req.Assist.ViewMode,
			},
			Onboard: &userpreferencesv1.OnboardUserPreferences{
				PreferredResources: req.Onboard.PreferredResources,
				MarketingParams: &userpreferencesv1.MarketingParams{
					Campaign: req.Onboard.MarketingParams.Campaign,
					Source:   req.Onboard.MarketingParams.Source,
					Medium:   req.Onboard.MarketingParams.Medium,
					Intent:   req.Onboard.MarketingParams.Intent,
				},
			},
			ClusterPreferences: &userpreferencesv1.ClusterUserPreferences{
				PinnedResources: &userpreferencesv1.PinnedResourcesUserPreferences{
					ResourceIds: req.ClusterPreferences.PinnedResources,
				},
			},
		},
	}
}

// updateUserPreferences is a handler for PUT /webapi/user/preferences.
func (h *Handler) updateUserPreferences(_ http.ResponseWriter, r *http.Request, _ httprouter.Params, sctx *SessionContext) (any, error) {
	var req UserPreferencesResponse

	if err := httplib.ReadJSON(r, &req); err != nil {
		return nil, trace.Wrap(err)
	}

	authClient, err := sctx.GetClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	preferences := makePreferenceRequest(req)
	if err := authClient.UpsertUserPreferences(r.Context(), preferences); err != nil {
		return nil, trace.Wrap(err)
	}

	return OK(), nil
}

// userPreferencesResponse creates a JSON response for the user preferences.
func userPreferencesResponse(resp *userpreferencesv1.UserPreferences) *UserPreferencesResponse {
	jsonResp := &UserPreferencesResponse{
		Assist:                     assistUserPreferencesResponse(resp.Assist),
		Theme:                      resp.Theme,
		Onboard:                    onboardUserPreferencesResponse(resp.Onboard),
		ClusterPreferences:         clusterPreferencesResponse(resp.ClusterPreferences),
		UnifiedResourcePreferences: unifiedResourcePreferencesResponse(resp.UnifiedResourcePreferences),
	}

	return jsonResp
}

func clusterPreferencesResponse(resp *userpreferencesv1.ClusterUserPreferences) ClusterUserPreferencesResponse {
	return ClusterUserPreferencesResponse{
		PinnedResources: resp.PinnedResources.ResourceIds,
	}
}

// assistUserPreferencesResponse creates a JSON response for the assist user preferences.
func assistUserPreferencesResponse(resp *userpreferencesv1.AssistUserPreferences) AssistUserPreferencesResponse {
	jsonResp := AssistUserPreferencesResponse{
		PreferredLogins: make([]string, 0, len(resp.PreferredLogins)),
		ViewMode:        resp.ViewMode,
	}

	jsonResp.PreferredLogins = append(jsonResp.PreferredLogins, resp.PreferredLogins...)

	return jsonResp
}

// unifiedResourcePreferencesResponse creates a JSON response for the assist user preferences.
func unifiedResourcePreferencesResponse(resp *userpreferencesv1.UnifiedResourcePreferences) UnifiedResourcePreferencesResponse {
	return UnifiedResourcePreferencesResponse{
		DefaultTab: resp.DefaultTab,
		ViewMode:   resp.ViewMode,
	}
}

// onboardUserPreferencesResponse creates a JSON response for the onboard user preferences.
func onboardUserPreferencesResponse(resp *userpreferencesv1.OnboardUserPreferences) OnboardUserPreferencesResponse {
	jsonResp := OnboardUserPreferencesResponse{
		PreferredResources: make([]userpreferencesv1.Resource, 0, len(resp.PreferredResources)),
		MarketingParams: preferencesMarketingParams{
			Campaign: resp.MarketingParams.Campaign,
			Source:   resp.MarketingParams.Source,
			Medium:   resp.MarketingParams.Medium,
			Intent:   resp.MarketingParams.Intent,
		},
	}

	jsonResp.PreferredResources = append(jsonResp.PreferredResources, resp.PreferredResources...)

	return jsonResp
}
