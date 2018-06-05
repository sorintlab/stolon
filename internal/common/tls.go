// Copyright 2016 Sorint.lab
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
// limitations under the License.

package common

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
)

func NewTLSConfig(certFile, keyFile, caFile string, insecureSkipVerify bool) (*tls.Config, error) {
	tlsConfig := tls.Config{}

	// Populate root CA certs
	if caFile != "" {
		pemBytes, err := ioutil.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		roots := x509.NewCertPool()

		for {
			var block *pem.Block
			block, pemBytes = pem.Decode(pemBytes)
			if block == nil {
				break
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			roots.AddCert(cert)
		}

		tlsConfig.RootCAs = roots
	}

	// Populate keypair
	// both must be defined
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	tlsConfig.InsecureSkipVerify = insecureSkipVerify

	return &tlsConfig, nil
}
