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

import "testing"

func TestRegisterConfig(t *testing.T) {
	t.Run("validate", func(t *testing.T) {
		t.Run("should check for consul register backend", func(t *testing.T) {
			config := Config{Backend: "something other than consul"}
			err := config.Validate()

			if err == nil || err.Error() != "unknown register backend: \"something other than consul\"" {
				t.Errorf("expected unknown register backend but got %s", err.Error())
			}
		})

		t.Run("should not return any error if all valid configurations are specified", func(t *testing.T) {
			config := Config{Backend: "consul"}
			err := config.Validate()

			if err != nil {
				t.Errorf("expected no error but got '%v'", err.Error())
			}
		})

		t.Run("should not support multiple addresses", func(t *testing.T) {
			config := Config{Backend: "consul", Endpoints: "http://127.0.0.1:8500,http://127.0.0.2:8500"}
			err := config.Validate()

			if err == nil || err.Error() != "consul does not support multiple endpoints: http://127.0.0.1:8500,http://127.0.0.2:8500" {
				t.Errorf("expected unknown register backend but got %s", err.Error())
			}
		})
	})

	t.Run("config", func(t *testing.T) {
		t.Run("should return config", func(t *testing.T) {
			c := Config{
				Backend:               "consul",
				Endpoints:             "https://127.0.0.1:8500",
				TLSAddress:            "address",
				TLSCAFile:             "ca-file",
				TLSCAPath:             "ca-path",
				TLSCertFile:           "cert-file",
				TLSKeyFile:            "key-file",
				TLSInsecureSkipVerify: true,
			}
			config, err := c.ConsulConfig()

			if err != nil {
				t.Errorf("expected error to be nil but got %s", err.Error())
			}

			if config.Address != "127.0.0.1:8500" {
				t.Errorf("expected address to be %s but got %s", c.Endpoints, config.Address)
			}

			if config.Scheme != "https" {
				t.Errorf("expected address to be https but got %s", config.Scheme)
			}

			if config.TLSConfig.Address != "address" {
				t.Errorf("expected tls address to be address but got %s", config.TLSConfig.Address)
			}

			if config.TLSConfig.CAFile != "ca-file" {
				t.Errorf("expected tlsCaFile to be ca-file but got %s", config.TLSConfig.CAFile)
			}

			if config.TLSConfig.CAPath != "ca-path" {
				t.Errorf("expected tlsCaPath to be ca-path but got %s", config.TLSConfig.CAPath)
			}

			if config.TLSConfig.CertFile != "cert-file" {
				t.Errorf("expected tlsCertFile to be cert-file but got %s", config.TLSConfig.CertFile)
			}

			if config.TLSConfig.KeyFile != "key-file" {
				t.Errorf("expected tlsKeyFile to be key-file but got %s", config.TLSConfig.KeyFile)
			}

			if config.TLSConfig.InsecureSkipVerify != true {
				t.Errorf("expected tlsInsecureSkipVerify to be true but got %v", config.TLSConfig.InsecureSkipVerify)
			}
		})
	})
}
