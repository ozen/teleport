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

// Package authclient contains common code for creating an auth server client
// which may use SSH tunneling through a proxy.
package authclient

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	"github.com/gravitational/teleport/api/breaker"
	apiclient "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/webclient"
	"github.com/gravitational/teleport/api/mfa"
	"github.com/gravitational/teleport/lib/auth"
	"github.com/gravitational/teleport/lib/reversetunnelclient"
	"github.com/gravitational/teleport/lib/utils"
)

// Config holds configuration parameters for connecting to the auth service.
type Config struct {
	// TLS holds credentials for mTLS.
	TLS *tls.Config
	// SSH is client SSH config.
	SSH *ssh.ClientConfig
	// AuthServers is a list of possible auth or proxy server addresses.
	AuthServers []utils.NetAddr
	// Log sets the logger for the client to use.
	Log logrus.FieldLogger
	// CircuitBreakerConfig is the configuration for the auth client circuit breaker.
	CircuitBreakerConfig breaker.Config
	// DialTimeout determines how long to wait for dialing to succeed before aborting.
	DialTimeout time.Duration
	// MFAPromptConstructor is used to create MFA prompts when needed.
	// If nil, the client will not prompt for MFA.
	MFAPromptConstructor mfa.PromptConstructor
	// Insecure turns off TLS certificate verification when enabled.
	Insecure bool
}

// Connect creates a valid client connection to the auth service.  It may
// connect directly to the auth server, or tunnel through the proxy.
func Connect(ctx context.Context, cfg *Config) (*auth.Client, error) {
	cfg.Log.Debugf("Connecting to: %v.", cfg.AuthServers)

	directClient, err := connectViaAuthDirect(ctx, cfg)
	if err == nil {
		return directClient, nil
	}
	directErr := trace.Wrap(err, "failed direct dial to auth server: %v", err)

	// If it fails, we now want to try tunneling to the auth server through a
	// proxy, we can only do this with SSH credentials.
	if cfg.SSH == nil {
		return nil, trace.Wrap(directErr)
	}
	proxyTunnelClient, err := connectViaProxyTunnel(ctx, cfg)
	if err == nil {
		return proxyTunnelClient, nil
	}
	proxyTunnelErr := trace.Wrap(err, "failed dial to auth server through reverse tunnel: %v", err)

	return nil, trace.NewAggregate(
		directErr,
		proxyTunnelErr,
	)
}

func connectViaAuthDirect(ctx context.Context, cfg *Config) (*auth.Client, error) {
	// Try connecting to the auth server directly over TLS.
	directClient, err := auth.NewClient(apiclient.Config{
		Addrs: utils.NetAddrsToStrings(cfg.AuthServers),
		Credentials: []apiclient.Credentials{
			apiclient.LoadTLS(cfg.TLS),
		},
		CircuitBreakerConfig:     cfg.CircuitBreakerConfig,
		InsecureAddressDiscovery: cfg.Insecure,
		DialTimeout:              cfg.DialTimeout,
		MFAPromptConstructor:     cfg.MFAPromptConstructor,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Check connectivity with a ping.
	if _, err := directClient.Ping(ctx); err != nil {
		// This client didn't work for us, so we close it.
		_ = directClient.Close()
		return nil, trace.Wrap(err)
	}

	return directClient, nil
}

func connectViaProxyTunnel(ctx context.Context, cfg *Config) (*auth.Client, error) {
	// If direct dial failed, we may have a proxy address in
	// cfg.AuthServers. Try connecting to the reverse tunnel
	// endpoint and make a client over that.
	//
	// TODO(nic): this logic should be implemented once and reused in IoT
	// nodes.
	resolver := reversetunnelclient.WebClientResolver(&webclient.Config{
		Context:   ctx,
		ProxyAddr: cfg.AuthServers[0].String(),
		Insecure:  cfg.Insecure,
		Timeout:   cfg.DialTimeout,
	})

	resolver, err := reversetunnelclient.CachingResolver(ctx, resolver, nil /* clock */)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// reversetunnel.TunnelAuthDialer will take care of creating a net.Conn
	// within an SSH tunnel.
	dialer, err := reversetunnelclient.NewTunnelAuthDialer(reversetunnelclient.TunnelAuthDialerConfig{
		Resolver:              resolver,
		ClientConfig:          cfg.SSH,
		Log:                   cfg.Log,
		InsecureSkipTLSVerify: cfg.Insecure,
		ClusterCAs:            cfg.TLS.RootCAs,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	tunnelClient, err := auth.NewClient(apiclient.Config{
		Dialer: dialer,
		Credentials: []apiclient.Credentials{
			apiclient.LoadTLS(cfg.TLS),
		},
		MFAPromptConstructor: cfg.MFAPromptConstructor,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Check connectivity with a ping.
	if _, err = tunnelClient.Ping(ctx); err != nil {
		// This client didn't work for us, so we close it.
		_ = tunnelClient.Close()
		return nil, trace.Wrap(err)
	}

	return tunnelClient, nil
}
