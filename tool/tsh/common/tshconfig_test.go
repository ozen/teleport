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

package common

import (
	"os"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/gravitational/teleport/api/profile"
	"github.com/gravitational/teleport/lib/utils"
)

func TestLoadConfigNonExistingFile(t *testing.T) {
	t.Parallel()

	fullFilePath := "/tmp/doesntexist." + uuid.NewString()
	gotConfig, gotErr := loadConfig(fullFilePath)
	require.NoError(t, gotErr)
	require.Equal(t, &TSHConfig{}, gotConfig)
}

func TestLoadConfigEmptyFile(t *testing.T) {
	t.Parallel()

	file, err := os.CreateTemp(t.TempDir(), "test-telelport")
	require.NoError(t, err)

	_, err = file.Write([]byte(" "))
	require.NoError(t, err)

	gotConfig, gotErr := loadConfig(file.Name())
	require.NoError(t, gotErr)
	require.Equal(t, &TSHConfig{}, gotConfig)
}

func TestLoadAllConfigs(t *testing.T) {
	t.Parallel()

	writeConf := func(fn string, config TSHConfig) {
		dir, _ := path.Split(fn)
		err := os.MkdirAll(dir, 0777)
		require.NoError(t, err)
		out, err := yaml.Marshal(config)
		require.NoError(t, err)
		err = os.WriteFile(fn, out, 0777)
		require.NoError(t, err)
	}

	tmp := t.TempDir()

	globalPath := path.Join(tmp, "etc", "tsh_global.yaml")
	globalConf := TSHConfig{
		ExtraHeaders: []ExtraProxyHeaders{{
			Proxy:   "global",
			Headers: map[string]string{"bar": "123"},
		}},
	}

	homeDir := path.Join(tmp, "home", "myuser", ".tsh")
	userPath := path.Join(homeDir, "config", "config.yaml")
	userConf := TSHConfig{
		ExtraHeaders: []ExtraProxyHeaders{{
			Proxy:   "user",
			Headers: map[string]string{"bar": "456"},
		}},
	}

	writeConf(globalPath, globalConf)
	writeConf(userPath, userConf)

	config, err := loadAllConfigs(CLIConf{
		GlobalTshConfigPath: globalPath,
		HomePath:            homeDir,
	})

	require.NoError(t, err)
	require.Equal(t, &TSHConfig{
		ExtraHeaders: []ExtraProxyHeaders{
			{
				Proxy:   "user",
				Headers: map[string]string{"bar": "456"},
			},
			{
				Proxy:   "global",
				Headers: map[string]string{"bar": "123"},
			},
		},
		Aliases: map[string]string{},
	}, config)

}

func TestTshConfigMerge(t *testing.T) {
	t.Parallel()

	sampleConfig := TSHConfig{
		ExtraHeaders: []ExtraProxyHeaders{{
			Proxy: "foo",
			Headers: map[string]string{
				"bar": "baz",
			},
		}},
		Aliases: map[string]string{},
	}

	tests := []struct {
		name    string
		config1 *TSHConfig
		config2 *TSHConfig
		want    TSHConfig
	}{
		{
			name:    "empty + empty = empty",
			config1: nil,
			config2: nil,
			want:    TSHConfig{Aliases: map[string]string{}},
		},
		{
			name:    "empty + x = x",
			config1: &sampleConfig,
			config2: nil,
			want:    sampleConfig,
		},
		{
			name:    "x + empty = x",
			config1: nil,
			config2: &sampleConfig,
			want:    sampleConfig,
		},
		{
			name: "headers combine different proxies",
			config1: &TSHConfig{
				ExtraHeaders: []ExtraProxyHeaders{{
					Proxy: "foo",
					Headers: map[string]string{
						"bar": "123",
					},
				}}},
			config2: &TSHConfig{
				ExtraHeaders: []ExtraProxyHeaders{{
					Proxy: "bar",
					Headers: map[string]string{
						"baz": "456",
					},
				}},
			},
			want: TSHConfig{
				ExtraHeaders: []ExtraProxyHeaders{
					{
						Proxy: "bar",
						Headers: map[string]string{
							"baz": "456",
						},
					},
					{
						Proxy: "foo",
						Headers: map[string]string{
							"bar": "123",
						},
					},
				},
				Aliases: map[string]string{},
			},
		},
		{
			name: "headers combine same proxy",
			config1: &TSHConfig{
				ExtraHeaders: []ExtraProxyHeaders{{
					Proxy: "foo",
					Headers: map[string]string{
						"bar": "123",
					},
				}}},
			config2: &TSHConfig{
				ExtraHeaders: []ExtraProxyHeaders{{
					Proxy: "foo",
					Headers: map[string]string{
						"bar": "456",
					},
				}}},
			want: TSHConfig{
				ExtraHeaders: []ExtraProxyHeaders{
					{
						Proxy: "foo",
						Headers: map[string]string{
							"bar": "456",
						},
					},
					{
						Proxy: "foo",
						Headers: map[string]string{
							"bar": "123",
						},
					},
				},
				Aliases: map[string]string{},
			},
		},
		{
			name: "aliases combine",
			config1: &TSHConfig{
				ExtraHeaders:   nil,
				ProxyTemplates: nil,
				Aliases: map[string]string{
					"foo": "foo1",
					"bar": "bar1",
				},
			},
			config2: &TSHConfig{
				ExtraHeaders:   nil,
				ProxyTemplates: nil,
				Aliases: map[string]string{
					"baz": "baz2",
					"bar": "bar2",
				},
			},
			want: TSHConfig{
				ExtraHeaders:   nil,
				ProxyTemplates: nil,
				Aliases: map[string]string{
					"foo": "foo1",
					"baz": "baz2",
					"bar": "bar2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config3 := tt.config1.Merge(tt.config2)
			require.Equal(t, tt.want, config3)
		})
	}
}

// TestProxyTemplatesApply verifies proxy templates matching functionality.
func TestProxyTemplatesApply(t *testing.T) {
	t.Parallel()

	tshConfig := TSHConfig{
		ProxyTemplates: ProxyTemplates{
			{
				Template: `^(.+)\.(us.example.com):(.+)$`,
				Proxy:    "$2:443",
				Cluster:  "$2",
				Host:     "$1:$3",
			},
			{
				Template: `^(.+)\.(eu.example.com):(.+)$`,
				Proxy:    "$2:3080",
			},
			{
				Template: `^(.+)\.(private-leaf):(.+)$`,
				Cluster:  "$2",
			},
			{
				Template: `^(.+)\.(au.example.com):(.+)$`,
				Host:     "$1:4022",
			},
		},
	}
	require.NoError(t, tshConfig.Check())

	tests := []struct {
		testName       string
		inFullHostname string
		outProxy       string
		outHost        string
		outCluster     string
		outMatch       bool
	}{
		{
			testName:       "matches first template",
			inFullHostname: "node-1.us.example.com:3022",
			outProxy:       "us.example.com:443",
			outCluster:     "us.example.com",
			outHost:        "node-1:3022",
			outMatch:       true,
		},
		{
			testName:       "matches second template",
			inFullHostname: "node-1.eu.example.com:3022",
			outProxy:       "eu.example.com:3080",
			outMatch:       true,
		},
		{
			testName:       "matches third template",
			inFullHostname: "node-1.private-leaf:3022",
			outCluster:     "private-leaf",
			outMatch:       true,
		},
		{
			testName:       "matches fourth template",
			inFullHostname: "node-1.au.example.com:3022",
			outHost:        "node-1:4022",
			outMatch:       true,
		},
		{
			testName:       "does not match templates",
			inFullHostname: "node-1.cn.example.com:3022",
			outMatch:       false,
		},
	}
	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			proxy, host, cluster, match := tshConfig.ProxyTemplates.Apply(test.inFullHostname)
			require.Equal(t, test.outProxy, proxy)
			require.Equal(t, test.outHost, host)
			require.Equal(t, test.outCluster, cluster)
			require.Equal(t, test.outMatch, match)
		})
	}
}

// TestProxyTemplates verifies proxy templates apply properly to client config.
func TestProxyTemplatesMakeClient(t *testing.T) {
	t.Parallel()

	tshConfig := TSHConfig{
		ProxyTemplates: ProxyTemplates{
			{
				Template: `^(.+)\.(us.example.com):(.+)$`,
				Proxy:    "$2:443",
				Cluster:  "$2",
				Host:     "$1:4022",
			},
		},
	}
	require.NoError(t, tshConfig.Check())

	newCLIConf := func(modify func(conf *CLIConf)) *CLIConf {
		// minimal configuration (with defaults)
		conf := &CLIConf{
			Proxy:     "proxy:3080",
			UserHost:  "localhost",
			HomePath:  t.TempDir(),
			TSHConfig: tshConfig,
		}

		// Create a empty profile so we don't ping proxy.
		clientStore, err := initClientStore(conf, conf.Proxy)
		require.NoError(t, err)
		profile := &profile.Profile{
			SSHProxyAddr: "proxy:3023",
			WebProxyAddr: "proxy:3080",
		}
		err = clientStore.SaveProfile(profile, true)
		require.NoError(t, err)

		modify(conf)
		return conf
	}

	for _, tt := range []struct {
		name         string
		InConf       *CLIConf
		expectErr    bool
		outHost      string
		outPort      int
		outCluster   string
		outJumpHosts []utils.JumpHost
	}{
		{
			name: "does not match template",
			InConf: newCLIConf(func(conf *CLIConf) {
				conf.UserHost = "node-1.cn.example.com:3022"
			}),
			outHost:    "node-1.cn.example.com:3022",
			outCluster: "proxy",
		},
		{
			name: "does not match template with -J {{proxy}}",
			InConf: newCLIConf(func(conf *CLIConf) {
				conf.UserHost = "node-1.cn.example.com:3022"
				conf.ProxyJump = "{{proxy}}"
			}),
			expectErr: true,
		},
		{
			name: "match with full host set",
			InConf: newCLIConf(func(conf *CLIConf) {
				conf.UserHost = "user@node-1.us.example.com:3022"
			}),
			outHost:    "node-1",
			outPort:    4022,
			outCluster: "us.example.com",
			outJumpHosts: []utils.JumpHost{{
				Addr: utils.NetAddr{
					Addr:        "us.example.com:443",
					AddrNetwork: "tcp",
				},
			}},
		},
		{
			name: "match with host and port set",
			InConf: newCLIConf(func(conf *CLIConf) {
				conf.UserHost = "user@node-1.us.example.com"
				conf.NodePort = 3022
			}),
			outHost:    "node-1",
			outPort:    4022,
			outCluster: "us.example.com",
			outJumpHosts: []utils.JumpHost{{
				Addr: utils.NetAddr{
					Addr:        "us.example.com:443",
					AddrNetwork: "tcp",
				},
			}},
		},
		{
			name: "match with -J {{proxy}} set",
			InConf: newCLIConf(func(conf *CLIConf) {
				conf.UserHost = "node-1.us.example.com:3022"
				conf.ProxyJump = "{{proxy}}"
			}),
			outHost:    "node-1",
			outPort:    4022,
			outCluster: "us.example.com",
			outJumpHosts: []utils.JumpHost{{
				Addr: utils.NetAddr{
					Addr:        "us.example.com:443",
					AddrNetwork: "tcp",
				},
			}},
		},
		{
			name: "match does not overwrite user specified proxy jump",
			InConf: newCLIConf(func(conf *CLIConf) {
				conf.UserHost = "node-1.us.example.com:3022"
				conf.SiteName = "specified.cluster"
				conf.ProxyJump = "specified.proxy.com:443"
			}),
			outHost:    "node-1",
			outPort:    4022,
			outCluster: "us.example.com",
			outJumpHosts: []utils.JumpHost{{
				Addr: utils.NetAddr{
					Addr:        "specified.proxy.com:443",
					AddrNetwork: "tcp",
				},
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tc, err := makeClient(tt.InConf)
			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.outHost, tc.Host)
			require.Equal(t, tt.outPort, tc.HostPort)
			require.Equal(t, tt.outJumpHosts, tc.JumpHosts)
			require.Equal(t, tt.outCluster, tc.SiteName)
		})
	}
}
