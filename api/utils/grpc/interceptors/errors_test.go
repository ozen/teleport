// Copyright 2023 Gravitational, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package interceptors_test

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/utils/grpc/interceptors"
)

type errService struct {
	proto.UnimplementedAuthServiceServer
}

func (s *errService) Ping(ctx context.Context, req *proto.PingRequest) (*proto.PingResponse, error) {
	return nil, trace.NotFound("not found")
}

func (s *errService) AddMFADevice(stream proto.AuthService_AddMFADeviceServer) error {
	return trace.AlreadyExists("already exists")
}

// TestGRPCErrorWrapping tests the error wrapping capability of the client
// and server unary and stream interceptors
func TestGRPCErrorWrapping(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptors.GRPCServerUnaryErrorInterceptor),
		grpc.ChainStreamInterceptor(interceptors.GRPCServerStreamErrorInterceptor),
	)
	proto.RegisterAuthServiceServer(server, &errService{})
	go func() {
		server.Serve(listener)
	}()
	defer server.Stop()

	conn, err := grpc.Dial(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(interceptors.GRPCClientUnaryErrorInterceptor),
		grpc.WithChainStreamInterceptor(interceptors.GRPCClientStreamErrorInterceptor),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := proto.NewAuthServiceClient(conn)

	t.Run("unary interceptor", func(t *testing.T) {
		resp, err := client.Ping(context.Background(), &proto.PingRequest{})
		require.Nil(t, resp)
		require.True(t, trace.IsNotFound(err))
		require.Equal(t, "not found", err.Error())
		_, ok := err.(*trace.TraceErr)
		require.False(t, ok, "client error should not include traces originating in the middleware")
	})

	t.Run("stream interceptor", func(t *testing.T) {
		//nolint:staticcheck // SA1019. The specific stream used here doesn't matter.
		stream, err := client.AddMFADevice(context.Background())
		require.NoError(t, err)

		//nolint:staticcheck // SA1019. The specific stream used here doesn't matter.
		sendErr := stream.Send(&proto.AddMFADeviceRequest{})

		// io.EOF means the server closed the stream, which can
		// happen depending in timing. In either case, it is
		// still safe to recv from the stream and check for
		// the already exists error.
		if sendErr != nil && !errors.Is(sendErr, io.EOF) {
			t.Fatalf("Unexpected error: %v", sendErr)
		}

		_, err = stream.Recv()
		require.True(t, trace.IsAlreadyExists(err))
		require.Equal(t, "already exists", err.Error())
		_, ok := err.(*trace.TraceErr)
		require.False(t, ok, "client error should not include traces originating in the middleware")
	})
}
