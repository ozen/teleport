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
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/gravitational/trace"
	"gopkg.in/yaml.v2"

	"github.com/gravitational/teleport/api/constants"
	"github.com/gravitational/teleport/api/profile"
)

// .tsh config must go in a subdir as all .yaml files in .tsh get
// parsed automatically by the profile loader and results in yaml
// unmarshal errors.
const tshConfigPath = "config/config.yaml"

// default location of global tsh config file.
const globalTshConfigPathDefault = "/etc/tsh.yaml"

// TSHConfig represents configuration loaded from the tsh config file.
type TSHConfig struct {
	// ExtraHeaders are additional http headers to be included in
	// webclient requests.
	ExtraHeaders []ExtraProxyHeaders `yaml:"add_headers,omitempty"`
	// ProxyTemplates describe rules for parsing out proxy out of full hostnames.
	ProxyTemplates ProxyTemplates `yaml:"proxy_templates,omitempty"`
	// Aliases are custom commands extending baseline tsh functionality.
	Aliases map[string]string `yaml:"aliases,omitempty"`
}

// Check validates the tsh config.
func (config *TSHConfig) Check() error {
	for _, template := range config.ProxyTemplates {
		if err := template.Check(); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// ExtraProxyHeaders represents the headers to include with the
// webclient.
type ExtraProxyHeaders struct {
	// Proxy is the domain of the proxy for these set of Headers, can contain globs.
	Proxy string `yaml:"proxy"`
	// Headers are the http header key values.
	Headers map[string]string `yaml:"headers,omitempty"`
}

// Merge two configs into one. The passed in otherConfig argument has higher priority.
func (config *TSHConfig) Merge(otherConfig *TSHConfig) TSHConfig {
	baseConfig := config
	if baseConfig == nil {
		baseConfig = &TSHConfig{}
	}

	if otherConfig == nil {
		otherConfig = &TSHConfig{}
	}

	newConfig := TSHConfig{}
	newConfig.ExtraHeaders = append(otherConfig.ExtraHeaders, baseConfig.ExtraHeaders...)
	newConfig.ProxyTemplates = append(otherConfig.ProxyTemplates, baseConfig.ProxyTemplates...)

	newConfig.Aliases = map[string]string{}
	for key, value := range baseConfig.Aliases {
		newConfig.Aliases[key] = value
	}
	for key, value := range otherConfig.Aliases {
		newConfig.Aliases[key] = value
	}

	return newConfig
}

// ProxyTemplates represents a list of individual proxy templates.
type ProxyTemplates []*ProxyTemplate

// Apply attempts to match the provided full hostname against all the templates
// in the list. Returns extracted proxy and host upon encountering the first
// matching template.
func (t ProxyTemplates) Apply(fullHostname string) (proxy, host, cluster string, matched bool) {
	for _, template := range t {
		proxy, host, cluster, matched := template.Apply(fullHostname)
		if matched {
			return proxy, host, cluster, true
		}
	}
	return "", "", "", false
}

// ProxyTemplate describes a single rule for parsing out proxy address from
// the full hostname. Used by tsh proxy ssh.
type ProxyTemplate struct {
	// Template is a regular expression that full hostname is matched against.
	Template string `yaml:"template"`
	// Proxy is the proxy address. Can refer to regex groups from the template.
	Proxy string `yaml:"proxy"`
	// Host is optional hostname. Can refer to regex groups from the template.
	Host string `yaml:"host"`
	// Cluster is optional cluster name. Can refer to regex groups from the template.
	Cluster string `yaml:"cluster"`
	// re is the compiled template regexp.
	re *regexp.Regexp
}

// Check validates the proxy template.
func (t *ProxyTemplate) Check() (err error) {
	if strings.TrimSpace(t.Template) == "" {
		return trace.BadParameter("empty proxy template")
	}
	if strings.TrimSpace(t.Proxy) == "" && strings.TrimSpace(t.Cluster) == "" && strings.TrimSpace(t.Host) == "" {
		return trace.BadParameter("empty proxy, cluster, and host fields in proxy template, but at least one is required")
	}
	t.re, err = regexp.Compile(t.Template)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// Apply applies the proxy template to the provided hostname and returns
// expanded proxy address and hostname.
func (t ProxyTemplate) Apply(fullHostname string) (proxy, host, cluster string, matched bool) {
	match := t.re.FindAllStringSubmatchIndex(fullHostname, -1)
	if match == nil {
		return "", "", "", false
	}

	if t.Proxy != "" {
		expandedProxy := []byte{}
		for _, m := range match {
			expandedProxy = t.re.ExpandString(expandedProxy, t.Proxy, fullHostname, m)
		}
		proxy = string(expandedProxy)
	}

	if t.Host != "" {
		expandedHost := []byte{}
		for _, m := range match {
			expandedHost = t.re.ExpandString(expandedHost, t.Host, fullHostname, m)
		}
		host = string(expandedHost)
	}

	if t.Cluster != "" {
		expandedCluster := []byte{}
		for _, m := range match {
			expandedCluster = t.re.ExpandString(expandedCluster, t.Cluster, fullHostname, m)
		}
		cluster = string(expandedCluster)
	}

	return proxy, host, cluster, true
}

// loadConfig load a single config file from given path. If the path does not exist, an empty config is returned instead.
func loadConfig(fullConfigPath string) (*TSHConfig, error) {
	bs, err := os.ReadFile(fullConfigPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &TSHConfig{}, nil
		}
		return nil, trace.ConvertSystemError(err)
	}
	cfg := TSHConfig{}
	if err := yaml.Unmarshal(bs, &cfg); err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	if err := cfg.Check(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &cfg, nil
}

// loadAllConfigs loads all tsh configs and merges them in appropriate order.
func loadAllConfigs(cf CLIConf) (*TSHConfig, error) {
	var globalConf *TSHConfig
	switch {
	// prefer using explicitly provided config paths
	case cf.GlobalTshConfigPath != "":
		cfg, err := loadConfig(cf.GlobalTshConfigPath)
		if err != nil {
			return nil, trace.Wrap(err, "failed to load global tsh config from %q", cf.GlobalTshConfigPath)
		}
		globalConf = cfg
	// skip the default global config path on windows see
	// teleport-private/#811 for more details
	case runtime.GOOS == constants.WindowsOS:
		globalConf = &TSHConfig{}
	// fallback to the global default on all other operating systems
	default:
		cfg, err := loadConfig(globalTshConfigPathDefault)
		if err != nil {
			return nil, trace.Wrap(err, "failed to load global tsh config from %q", globalTshConfigPathDefault)
		}
		globalConf = cfg
	}

	fullConfigPath := filepath.Join(profile.FullProfilePath(cf.HomePath), tshConfigPath)
	userConf, err := loadConfig(fullConfigPath)
	if err != nil {
		return nil, trace.Wrap(err, "failed to load tsh config from %q", fullConfigPath)
	}

	confOptions := globalConf.Merge(userConf)
	return &confOptions, nil
}
