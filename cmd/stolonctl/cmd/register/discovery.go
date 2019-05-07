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

package register

import (
	"errors"

	"github.com/hashicorp/consul/api"
)

// ServiceDiscovery helps to register service
type ServiceDiscovery interface {
	Register(info *ServiceInfo) error
	Services(name string) (ServiceInfos, error)
	DeRegister(info *ServiceInfo) error
}

// NewServiceDiscovery creates a Discovery from registerBackend and registerEndpoints
func NewServiceDiscovery(config *Config) (ServiceDiscovery, error) {
	switch config.Backend {
	case "consul":
		if apiConfig, err := config.ConsulConfig(); err != nil {
			return nil, err
		} else if client, err := api.NewClient(apiConfig); err != nil {
			return nil, err
		} else {
			agent := client.Agent()
			catalog := client.Catalog()
			return NewConsulServiceDiscovery(agent, catalog), nil
		}
	default:
		return nil, errors.New("register backend not supported")
	}
}

// ConsulServiceDiscovery helps to register service to consul
type ConsulServiceDiscovery struct {
	agent   ConsulAgent
	catalog ConsulCatalog
}

// ConsulAgent interface holds all the necessary methods to interact with consul agent
type ConsulAgent interface {
	ServiceRegister(service *api.AgentServiceRegistration) error
	ServiceDeregister(serviceID string) error
}

type ConsulCatalog interface {
	Service(service, tag string, q *api.QueryOptions) ([]*api.CatalogService, *api.QueryMeta, error)
}

// NewConsulServiceDiscovery creates a new ConsulDiscovery
func NewConsulServiceDiscovery(agent ConsulAgent, catalog ConsulCatalog) ServiceDiscovery {
	return &ConsulServiceDiscovery{agent: agent, catalog: catalog}
}

// Register registers the given service info to consul
func (cd *ConsulServiceDiscovery) Register(info *ServiceInfo) error {
	return cd.agent.ServiceRegister(info.ConsulAgentServiceRegistration())
}

// DeRegister de-registers the given service info to consul
func (cd *ConsulServiceDiscovery) DeRegister(info *ServiceInfo) error {
	return cd.agent.ServiceDeregister(info.ID)
}

// Services returns the services registered in the consul
func (cd *ConsulServiceDiscovery) Services(name string) (ServiceInfos, error) {
	services, _, err := cd.catalog.Service(name, "", nil)
	if err != nil {
		return nil, err
	}
	result := ServiceInfos{}
	for _, service := range services {
		if service != nil {
			consulService := NewServiceInfoFromConsulService(*service)
			result[service.ServiceID] = consulService
		}
	}
	return result, nil
}
