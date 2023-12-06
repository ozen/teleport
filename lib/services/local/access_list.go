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

package local

import (
	"context"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"

	accesslistv1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/accesslist/v1"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/accesslist"
	"github.com/gravitational/teleport/api/types/header"
	"github.com/gravitational/teleport/lib/backend"
	"github.com/gravitational/teleport/lib/modules"
	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/services/local/generic"
)

const (
	accessListPrefix      = "access_list"
	accessListMaxPageSize = 100

	accessListMemberPrefix      = "access_list_member"
	accessListMemberMaxPageSize = 200

	accessListReviewPrefix      = "access_list_review"
	accessListReviewMaxPageSize = 200

	// This lock is necessary to prevent a race condition between access lists and members and to ensure
	// consistency of the one-to-many relationship between them.
	accessListLockTTL = 5 * time.Second
)

// AccessListService manages Access List resources in the Backend.
type AccessListService struct {
	log           logrus.FieldLogger
	clock         clockwork.Clock
	service       *generic.Service[*accesslist.AccessList]
	memberService *generic.Service[*accesslist.AccessListMember]
	reviewService *generic.Service[*accesslist.Review]
}

// NewAccessListService creates a new AccessListService.
func NewAccessListService(backend backend.Backend, clock clockwork.Clock) (*AccessListService, error) {
	service, err := generic.NewService(&generic.ServiceConfig[*accesslist.AccessList]{
		Backend:       backend,
		PageLimit:     accessListMaxPageSize,
		ResourceKind:  types.KindAccessList,
		BackendPrefix: accessListPrefix,
		MarshalFunc:   services.MarshalAccessList,
		UnmarshalFunc: services.UnmarshalAccessList,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	memberService, err := generic.NewService(&generic.ServiceConfig[*accesslist.AccessListMember]{
		Backend:       backend,
		PageLimit:     accessListMemberMaxPageSize,
		ResourceKind:  types.KindAccessListMember,
		BackendPrefix: accessListMemberPrefix,
		MarshalFunc:   services.MarshalAccessListMember,
		UnmarshalFunc: services.UnmarshalAccessListMember,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	reviewService, err := generic.NewService(&generic.ServiceConfig[*accesslist.Review]{
		Backend:       backend,
		PageLimit:     accessListReviewMaxPageSize,
		ResourceKind:  types.KindAccessListReview,
		BackendPrefix: accessListReviewPrefix,
		MarshalFunc:   services.MarshalAccessListReview,
		UnmarshalFunc: services.UnmarshalAccessListReview,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &AccessListService{
		log:           logrus.WithFields(logrus.Fields{trace.Component: "access-list:local-service"}),
		clock:         clock,
		service:       service,
		memberService: memberService,
		reviewService: reviewService,
	}, nil
}

// GetAccessLists returns a list of all access lists.
func (a *AccessListService) GetAccessLists(ctx context.Context) ([]*accesslist.AccessList, error) {
	accessLists, err := a.service.GetResources(ctx)
	return accessLists, trace.Wrap(err)
}

// ListAccessLists returns a paginated list of access lists.
func (a *AccessListService) ListAccessLists(ctx context.Context, pageSize int, nextToken string) ([]*accesslist.AccessList, string, error) {
	return a.service.ListResources(ctx, pageSize, nextToken)
}

// GetAccessList returns the specified access list resource.
func (a *AccessListService) GetAccessList(ctx context.Context, name string) (*accesslist.AccessList, error) {
	var accessList *accesslist.AccessList
	err := a.service.RunWhileLocked(ctx, lockName(name), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		var err error
		accessList, err = a.service.GetResource(ctx, name)
		return trace.Wrap(err)
	})
	return accessList, trace.Wrap(err)
}

// GetAccessListsToReview returns access lists that the user needs to review. This is not implemented in the local service.
func (a *AccessListService) GetAccessListsToReview(ctx context.Context) ([]*accesslist.AccessList, error) {
	return nil, trace.NotImplemented("GetAccessListsToReview should not be called")
}

// UpsertAccessList creates or updates an access list resource.
func (a *AccessListService) UpsertAccessList(ctx context.Context, accessList *accesslist.AccessList) (*accesslist.AccessList, error) {
	upsertWithLockFn := func() error {
		return a.service.RunWhileLocked(ctx, lockName(accessList.GetName()), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
			ownerMap := make(map[string]struct{}, len(accessList.Spec.Owners))
			for _, owner := range accessList.Spec.Owners {
				if _, ok := ownerMap[owner.Name]; ok {
					return trace.AlreadyExists("owner %s already exists in the owner list", owner.Name)
				}
				ownerMap[owner.Name] = struct{}{}
			}
			return trace.Wrap(a.service.UpsertResource(ctx, accessList))
		})
	}

	var err error
	if feature := modules.GetModules().Features(); !feature.IGSEnabled() {
		err = a.service.RunWhileLocked(ctx, "createAccessListLimitLock", accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
			if err := a.VerifyAccessListCreateLimit(ctx, accessList.GetName()); err != nil {
				return trace.Wrap(err)
			}
			return trace.Wrap(upsertWithLockFn())
		})
	} else {
		err = upsertWithLockFn()
	}

	if err != nil {
		return nil, trace.Wrap(err)
	}

	return accessList, nil
}

// DeleteAccessList removes the specified access list resource.
func (a *AccessListService) DeleteAccessList(ctx context.Context, name string) error {
	err := a.service.RunWhileLocked(ctx, lockName(name), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		// Delete all associated members.
		err := a.memberService.WithPrefix(name).DeleteAllResources(ctx)
		if err != nil {
			return trace.Wrap(err)
		}

		return trace.Wrap(a.service.DeleteResource(ctx, name))
	})

	return trace.Wrap(err)
}

// DeleteAllAccessLists removes all access lists.
func (a *AccessListService) DeleteAllAccessLists(ctx context.Context) error {
	// Locks are not used here as these operations are more likely to be used by the cache.
	// Delete all members for all access lists.
	err := a.memberService.DeleteAllResources(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(a.service.DeleteAllResources(ctx))
}

// ListAccessListMembers returns a paginated list of all access list members.
func (a *AccessListService) ListAccessListMembers(ctx context.Context, accessList string, pageSize int, nextToken string) ([]*accesslist.AccessListMember, string, error) {
	var members []*accesslist.AccessListMember
	err := a.service.RunWhileLocked(ctx, lockName(accessList), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		_, err := a.service.GetResource(ctx, accessList)
		if err != nil {
			return trace.Wrap(err)
		}
		members, nextToken, err = a.memberService.WithPrefix(accessList).ListResources(ctx, pageSize, nextToken)
		return trace.Wrap(err)
	})
	if err != nil {
		return nil, "", trace.Wrap(err)
	}
	return members, nextToken, nil
}

// GetAccessListMember returns the specified access list member resource.
func (a *AccessListService) GetAccessListMember(ctx context.Context, accessList string, memberName string) (*accesslist.AccessListMember, error) {
	var member *accesslist.AccessListMember
	err := a.service.RunWhileLocked(ctx, lockName(accessList), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		_, err := a.service.GetResource(ctx, accessList)
		if err != nil {
			return trace.Wrap(err)
		}
		member, err = a.memberService.WithPrefix(accessList).GetResource(ctx, memberName)
		return trace.Wrap(err)
	})
	return member, trace.Wrap(err)
}

// UpsertAccessListMember creates or updates an access list member resource.
func (a *AccessListService) UpsertAccessListMember(ctx context.Context, member *accesslist.AccessListMember) (*accesslist.AccessListMember, error) {
	err := a.service.RunWhileLocked(ctx, lockName(member.Spec.AccessList), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		_, err := a.service.GetResource(ctx, member.Spec.AccessList)
		if err != nil {
			return trace.Wrap(err)
		}
		return trace.Wrap(a.memberService.WithPrefix(member.Spec.AccessList).UpsertResource(ctx, member))
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return member, nil
}

// DeleteAccessListMember hard deletes the specified access list member resource.
func (a *AccessListService) DeleteAccessListMember(ctx context.Context, accessList string, memberName string) error {
	err := a.service.RunWhileLocked(ctx, lockName(accessList), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		_, err := a.service.GetResource(ctx, accessList)
		if err != nil {
			return trace.Wrap(err)
		}
		return trace.Wrap(a.memberService.WithPrefix(accessList).DeleteResource(ctx, memberName))
	})
	return trace.Wrap(err)
}

// DeleteAllAccessListMembersForAccessList hard deletes all access list members for an access list.
func (a *AccessListService) DeleteAllAccessListMembersForAccessList(ctx context.Context, accessList string) error {
	err := a.service.RunWhileLocked(ctx, lockName(accessList), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		_, err := a.service.GetResource(ctx, accessList)
		if err != nil {
			return trace.Wrap(err)
		}
		return trace.Wrap(a.memberService.WithPrefix(accessList).DeleteAllResources(ctx))
	})
	return trace.Wrap(err)
}

// DeleteAllAccessListMembers hard deletes all access list members.
func (a *AccessListService) DeleteAllAccessListMembers(ctx context.Context) error {

	// Locks are not used here as this operation is more likely to be used by the cache.
	return trace.Wrap(a.memberService.DeleteAllResources(ctx))
}

// UpsertAccessListWithMembers creates or updates an access list resource and its members.
func (a *AccessListService) UpsertAccessListWithMembers(ctx context.Context, accessList *accesslist.AccessList, membersIn []*accesslist.AccessListMember) (*accesslist.AccessList, []*accesslist.AccessListMember, error) {
	// Double the lock TTL to account for the time it takes to upsert the members.
	upsertWithLockFn := func() error {
		return a.service.RunWhileLocked(ctx, lockName(accessList.GetName()), 2*accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
			// Create a map of the members from the request for easier lookup.
			membersMap := make(map[string]*accesslist.AccessListMember)

			// Convert the members slice to a map for easier lookup.
			for _, member := range membersIn {
				membersMap[member.GetName()] = member
			}

			var (
				members      []*accesslist.AccessListMember
				membersToken string
				err          error
			)

			for {
				// List all members for the access list.
				members, membersToken, err = a.memberService.WithPrefix(accessList.GetName()).ListResources(ctx, 0 /* default size */, membersToken)
				if err != nil {
					return trace.Wrap(err)
				}

				for _, member := range members {
					// If the member is not in the members map (request), delete it.
					if _, ok := membersMap[member.GetName()]; !ok {
						err = a.memberService.WithPrefix(accessList.GetName()).DeleteResource(ctx, member.GetName())
						if err != nil {
							return trace.Wrap(err)
						}
					} else {
						// Compare members and update if necessary.
						if !cmp.Equal(member, membersMap[member.GetName()]) {
							// Update the member.
							err = a.memberService.WithPrefix(accessList.GetName()).UpsertResource(ctx, membersMap[member.GetName()])
							if err != nil {
								return trace.Wrap(err)
							}
						}
					}

					// Remove the member from the map.
					delete(membersMap, member.GetName())
				}

				if membersToken == "" {
					break
				}
			}

			// Add any remaining members to the access list.
			for _, member := range membersMap {
				err = a.memberService.WithPrefix(accessList.GetName()).UpsertResource(ctx, member)
				if err != nil {
					return trace.Wrap(err)
				}
			}

			return trace.Wrap(a.service.UpsertResource(ctx, accessList))
		})
	}

	var err error
	if feature := modules.GetModules().Features(); !feature.IGSEnabled() {
		err = a.service.RunWhileLocked(ctx, "createAccessListWithMembersLimitLock", accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
			if err := a.VerifyAccessListCreateLimit(ctx, accessList.GetName()); err != nil {
				return trace.Wrap(err)
			}
			return trace.Wrap(upsertWithLockFn())
		})
	} else {
		err = upsertWithLockFn()
	}

	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	return accessList, membersIn, nil
}

func (a *AccessListService) AccessRequestPromote(_ context.Context, _ *accesslistv1.AccessRequestPromoteRequest) (*accesslistv1.AccessRequestPromoteResponse, error) {
	return nil, trace.NotImplemented("AccessRequestPromote should not be called")
}

// ListAccessListReviews will list access list reviews for a particular access list.
func (a *AccessListService) ListAccessListReviews(ctx context.Context, accessList string, pageSize int, pageToken string) (reviews []*accesslist.Review, nextToken string, err error) {
	err = a.service.RunWhileLocked(ctx, lockName(accessList), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		_, err := a.service.GetResource(ctx, accessList)
		if err != nil {
			return trace.Wrap(err)
		}
		reviews, nextToken, err = a.reviewService.WithPrefix(accessList).ListResources(ctx, pageSize, pageToken)
		return trace.Wrap(err)
	})
	if err != nil {
		return nil, "", trace.Wrap(err)
	}
	return reviews, nextToken, nil
}

// CreateAccessListReview will create a new review for an access list.
func (a *AccessListService) CreateAccessListReview(ctx context.Context, review *accesslist.Review) (*accesslist.Review, time.Time, error) {
	reviewName := uuid.New().String()
	createdReview, err := accesslist.NewReview(header.Metadata{
		Name: reviewName,
	}, accesslist.ReviewSpec{
		AccessList: review.Spec.AccessList,
		Reviewers:  review.Spec.Reviewers,
		ReviewDate: review.Spec.ReviewDate,
		Changes:    review.Spec.Changes,
	})
	if err != nil {
		return nil, time.Time{}, trace.Wrap(err)
	}

	var nextAuditDate time.Time

	err = a.service.RunWhileLocked(ctx, lockName(review.Spec.AccessList), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		accessList, err := a.service.GetResource(ctx, review.Spec.AccessList)
		if err != nil {
			return trace.Wrap(err)
		}

		if createdReview.Spec.Changes.MembershipRequirementsChanged != nil {
			if accessListRequiresEqual(*createdReview.Spec.Changes.MembershipRequirementsChanged, accessList.Spec.MembershipRequires) {
				createdReview.Spec.Changes.MembershipRequirementsChanged = nil
			} else {
				accessList.Spec.MembershipRequires = *review.Spec.Changes.MembershipRequirementsChanged
			}
		}

		if createdReview.Spec.Changes.ReviewFrequencyChanged != 0 {
			if createdReview.Spec.Changes.ReviewFrequencyChanged == accessList.Spec.Audit.Recurrence.Frequency {
				createdReview.Spec.Changes.ReviewFrequencyChanged = 0
			} else {
				accessList.Spec.Audit.Recurrence.Frequency = review.Spec.Changes.ReviewFrequencyChanged
			}
		}

		if createdReview.Spec.Changes.ReviewDayOfMonthChanged != 0 {
			if createdReview.Spec.Changes.ReviewDayOfMonthChanged == accessList.Spec.Audit.Recurrence.DayOfMonth {
				createdReview.Spec.Changes.ReviewDayOfMonthChanged = 0
			} else {
				accessList.Spec.Audit.Recurrence.DayOfMonth = review.Spec.Changes.ReviewDayOfMonthChanged
			}
		}

		if err := a.reviewService.WithPrefix(review.Spec.AccessList).CreateResource(ctx, createdReview); err != nil {
			return trace.Wrap(err)
		}

		nextAuditDate = services.SelectNextReviewDate(accessList)
		accessList.Spec.Audit.NextAuditDate = nextAuditDate

		for _, removedMember := range review.Spec.Changes.RemovedMembers {
			if err := a.memberService.WithPrefix(review.Spec.AccessList).DeleteResource(ctx, removedMember); err != nil {
				return trace.Wrap(err)
			}
		}

		if err := a.service.UpdateResource(ctx, accessList); err != nil {
			return trace.Wrap(err, "updating audit date in access list")
		}

		return nil
	})
	if err != nil {
		return nil, time.Time{}, trace.Wrap(err)
	}

	return createdReview, nextAuditDate, nil
}

// accessListRequiresEqual returns true if two access lists are equal.
func accessListRequiresEqual(a, b accesslist.Requires) bool {
	// Check roles and traits length.
	if len(a.Roles) != len(b.Roles) {
		return false
	}
	if len(a.Traits) != len(b.Traits) {
		return false
	}

	// Make sure roles are equal.
	for i, role := range a.Roles {
		if b.Roles[i] != role {
			return false
		}
	}

	// Make sure traits are equal.
	for key, vals := range a.Traits {
		bVals, ok := b.Traits[key]
		if !ok {
			return false
		}

		if len(bVals) != len(vals) {
			return false
		}

		for i, val := range vals {
			if bVals[i] != val {
				return false
			}
		}
	}

	return true
}

// DeleteAccessListReview will delete an access list review from the backend.
func (a *AccessListService) DeleteAccessListReview(ctx context.Context, accessListName, reviewName string) error {
	err := a.service.RunWhileLocked(ctx, lockName(accessListName), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		_, err := a.service.GetResource(ctx, accessListName)
		if err != nil {
			return trace.Wrap(err)
		}
		return trace.Wrap(a.reviewService.WithPrefix(accessListName).DeleteResource(ctx, reviewName))
	})
	return trace.Wrap(err)
}

// DeleteAllAccessListReviews will delete all access list reviews from an access list.
func (a *AccessListService) DeleteAllAccessListReviews(ctx context.Context, accessList string) error {
	err := a.service.RunWhileLocked(ctx, lockName(accessList), accessListLockTTL, func(ctx context.Context, _ backend.Backend) error {
		_, err := a.service.GetResource(ctx, accessList)
		if err != nil {
			return trace.Wrap(err)
		}
		return trace.Wrap(a.reviewService.WithPrefix(accessList).DeleteAllResources(ctx))
	})
	return trace.Wrap(err)
}

func lockName(accessListName string) string {
	return strings.Join([]string{"access_list", accessListName}, string(backend.Separator))
}

// VerifyAccessListCreateLimit ensures creating access list is limited to no more than 1 (updating is allowed).
// It differentiates request for `creating` and `updating` by checking to see if the request
// access list name matches the ones we retrieved.
// Returns error if limit has been reached.
func (a *AccessListService) VerifyAccessListCreateLimit(ctx context.Context, targetAccessListName string) error {
	feature := modules.GetModules().Features()
	if feature.IGSEnabled() {
		return nil // unlimited
	}

	lists, err := a.GetAccessLists(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	if len(lists) == 0 {
		return nil
	}

	// Iterate through fetched lists, to check if the request was
	// an update, which is allowed.
	for _, list := range lists {
		if list.GetName() == targetAccessListName {
			return nil
		}
	}

	if len(lists) < feature.AccessList.CreateLimit {
		return nil
	}

	const limitReachedMessage = "cluster has reached its limit for creating access lists, please contact the cluster administrator"
	return trace.AccessDenied(limitReachedMessage)
}
