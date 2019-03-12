// Copyright 2019 Sorint.lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and

package register

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/consul/api"
)

// Config represents necessary configurations which can passed
// for registering master and slave info for service discovery
type Config struct {
	Backend               string
	Endpoints             string
	Token                 string
	TLSAddress            string
	TLSCAFile             string
	TLSCAPath             string
	TLSCertFile           string
	TLSKeyFile            string
	TLSInsecureSkipVerify bool
	SleepInterval         int
	TagMasterAs           string
	TagSlaveAs            string
	RegisterMaster        bool
}

// Validate returns nil if the config is valid, else returns error with
// appropriate reason
func (config *Config) Validate() error {
	switch config.Backend {
	case "consul":
		addresses := strings.Split(config.Endpoints, ",")
		if len(addresses) != 1 {
			return fmt.Errorf("consul does not support multiple endpoints: %s", config.Endpoints)
		}
		_, err := url.Parse(config.Endpoints)
		return err
	default:
		return fmt.Errorf("unknown register backend: %q", config.Backend)
	}
}

// ConsulConfig returns consul.api.ConsulConfig if register endpoint is valid consul url
// else will return error with appropriate reason
func (config *Config) ConsulConfig() (*api.Config, error) {
	url, err := url.Parse(config.Endpoints)
	if err != nil {
		return nil, err
	}
	return &api.Config{
		Address: url.Host,
		Scheme:  url.Scheme,
		TLSConfig: api.TLSConfig{
			Address:            config.TLSAddress,
			CAFile:             config.TLSCAFile,
			CAPath:             config.TLSCAPath,
			CertFile:           config.TLSCertFile,
			KeyFile:            config.TLSKeyFile,
			InsecureSkipVerify: config.TLSInsecureSkipVerify,
		},
	}, nil
}
