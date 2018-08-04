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
// limitations under the License.

package register_test

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul/api"
	"github.com/sorintlab/stolon/cmd/stolonctl/cmd/internal/mock/register"
	"github.com/sorintlab/stolon/cmd/stolonctl/cmd/register"
)

func TestNewServiceDiscovery(t *testing.T) {
	t.Run("should return consul service discovery", func(t *testing.T) {
		config := register.Config{Backend: "consul", Endpoints: "http://127.0.0.1"}
		sd, err := register.NewServiceDiscovery(&config)

		if err != nil {
			t.Errorf("expected error to be nil but was %s", err.Error())
		} else if sd == nil {
			t.Errorf("expected service discovery not to be nil")
		}
	})
}

func TestConsulServiceDiscovery(t *testing.T) {
	t.Run("should be able to register service info", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client := mock_register.NewMockConsulAgent(ctrl)
		catalog := mock_register.NewMockConsulCatalog(ctrl)
		serviceDiscovery := register.NewConsulServiceDiscovery(client, catalog)
		expectedServiceInfo := register.ServiceInfo{
			Name:    "service",
			ID:      "1",
			Port:    5432,
			Address: "127.0.0.1",
			Tags:    []string{"tag"},
			Check: register.HealthCheck{
				Interval: "10s",
				TCP:      "tcp",
			},
		}

		client.EXPECT().ServiceRegister(expectedServiceInfo.ConsulAgentServiceRegistration())

		err := serviceDiscovery.Register(&expectedServiceInfo)

		if err != nil {
			t.Errorf("expected error to be nil when registering service but got %s", err.Error())
		}
	})

	t.Run("should return error returned by http client", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client := mock_register.NewMockConsulAgent(ctrl)
		catalog := mock_register.NewMockConsulCatalog(ctrl)
		serviceDiscovery := register.NewConsulServiceDiscovery(client, catalog)
		client.EXPECT().ServiceRegister(gomock.Any()).Return(errors.New("something went wrong"))

		err := serviceDiscovery.Register(&register.ServiceInfo{})

		if err == nil || err.Error() != "something went wrong" {
			t.Errorf("expected error to be something went wrong")
		}
	})
}

func TestServices(t *testing.T) {
	t.Run("should return valid service infos", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client := mock_register.NewMockConsulAgent(ctrl)
		catalog := mock_register.NewMockConsulCatalog(ctrl)
		serviceDiscovery := register.NewConsulServiceDiscovery(client, catalog)
		services := []*api.CatalogService{
			{ServiceID: "masterUID", ServiceName: "test"},
			{ServiceID: "slaveUID", ServiceName: "test"},
			nil,
		}

		catalog.EXPECT().Service("test", "", nil).Return(services, nil, nil)

		infos, err := serviceDiscovery.Services("test")

		if err != nil {
			t.Errorf("expected error to be nil but was %s", err.Error())
		}
		if len(infos) != 2 {
			t.Errorf("expected 2 services but got %d", len(infos))
		}
	})

	t.Run("should return error if service returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		client := mock_register.NewMockConsulAgent(ctrl)
		catalog := mock_register.NewMockConsulCatalog(ctrl)
		serviceDiscovery := register.NewConsulServiceDiscovery(client, catalog)

		catalog.EXPECT().Service("test", "", nil).Return(nil, nil, errors.New("service error"))

		infos, err := serviceDiscovery.Services("test")

		if err == nil {
			t.Errorf("expected error to be service error but got nil")
		}
		if infos != nil {
			t.Errorf("expected 2 services but got %d", len(infos))
		}
	})
}
