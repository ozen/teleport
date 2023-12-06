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

package aggregating

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	prehogv1 "github.com/gravitational/teleport/gen/proto/go/prehog/v1"
	"github.com/gravitational/teleport/lib/backend/memory"
)

func newReport(startTime time.Time) *prehogv1.UserActivityReport {
	u := uuid.New()
	r := &prehogv1.UserActivityReport{
		ReportUuid: u[:],
		StartTime:  timestamppb.New(startTime),
	}
	return r
}

func TestCRUD(t *testing.T) {
	ctx := context.Background()
	clk := clockwork.NewFakeClock()
	bk, err := memory.New(memory.Config{
		Clock:     clk,
		EventsOff: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, bk.Close()) })

	svc := reportService{bk}

	r0 := newReport(clk.Now().Add(time.Minute))
	r1 := newReport(clk.Now().Add(time.Minute))
	r2 := newReport(clk.Now().Add(2 * time.Minute))

	require.NoError(t, svc.upsertUserActivityReport(ctx, r0, time.Hour))
	require.NoError(t, svc.upsertUserActivityReport(ctx, r1, time.Hour))
	require.NoError(t, svc.upsertUserActivityReport(ctx, r2, time.Hour))

	// we expect r0 and r1 in unspecified order
	reports, err := svc.listUserActivityReports(ctx, 2)
	require.NoError(t, err)
	require.Len(t, reports, 2)
	if proto.Equal(r0, reports[0]) {
		require.True(t, proto.Equal(r1, reports[1]))
	} else {
		require.True(t, proto.Equal(r0, reports[1]))
		require.True(t, proto.Equal(r1, reports[0]))
	}

	require.NoError(t, svc.deleteUserActivityReport(ctx, r1))
	reports, err = svc.listUserActivityReports(ctx, 5)
	require.NoError(t, err)
	require.Len(t, reports, 2)
	// r2 has a later start time, so it must appear later
	require.True(t, proto.Equal(r0, reports[0]))
	require.True(t, proto.Equal(r2, reports[1]))

	clk.Advance(time.Minute + time.Hour)
	reports, err = svc.listUserActivityReports(ctx, 5)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	require.True(t, proto.Equal(r2, reports[0]))
}

func TestLock(t *testing.T) {
	ctx := context.Background()
	clk := clockwork.NewFakeClock()
	bk, err := memory.New(memory.Config{
		Clock:     clk,
		EventsOff: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, bk.Close()) })

	svc := reportService{bk}

	require.NoError(t, svc.createUserActivityReportsLock(ctx, 2*time.Minute, nil))
	clk.Advance(time.Minute)
	err = svc.createUserActivityReportsLock(ctx, 2*time.Minute, nil)
	require.Error(t, err)
	require.True(t, trace.IsAlreadyExists(err))
	clk.Advance(time.Minute)
	require.NoError(t, svc.createUserActivityReportsLock(ctx, 2*time.Minute, nil))
}
