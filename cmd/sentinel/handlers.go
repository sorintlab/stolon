// Copyright 2015 Sorint.lab
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

package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sorintlab/stolon/pkg/cluster"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
)

func (s *Sentinel) updateConfigHandler(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	vars := mux.Vars(req)
	configName := vars["name"]

	// only configID == current is currently supported
	if configName != "current" {
		log.Errorf("wrong config name %q", configName)
		http.Error(w, fmt.Sprintf("wrong config name %q", configName), http.StatusBadRequest)

	}

	decoder := json.NewDecoder(req.Body)
	var config *cluster.NilConfig
	err := decoder.Decode(&config)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}

	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()

	if !s.isLeader() {
		log.Errorf("we aren't the sentinels leader. cannot process config update request.")
		http.Error(w, "we aren't the sentinels leader. cannot process config update request.", http.StatusBadRequest)
	}

	log.Infof(spew.Sprintf("updating config to %#v", config))

	e := s.e

	cd, pair, err := e.GetClusterData()
	if err != nil {
		log.Errorf("error retrieving cluster data: %v", err)
		http.Error(w, fmt.Sprintf("error retrieving cluster data: %v", err), http.StatusInternalServerError)
	}

	if cd == nil {
		log.Errorf("empty cluster data")
		http.Error(w, "empty cluster data", http.StatusInternalServerError)
	}
	if cd.ClusterView == nil {
		log.Errorf("empty cluster view")
		http.Error(w, "empty cluster view", http.StatusInternalServerError)
	}
	log.Debugf(spew.Sprintf("keepersState: %#v", cd.KeepersState))
	log.Debugf(spew.Sprintf("clusterView: %#v", cd.ClusterView))

	newcv := cd.ClusterView.Copy()
	newcv.Config = config
	newcv.Version += 1
	if _, err := e.SetClusterData(cd.KeepersState, newcv, pair); err != nil {
		log.Errorf("error saving clusterdata: %v", err)
		http.Error(w, fmt.Sprintf("error saving clusterdata: %v", err), http.StatusInternalServerError)
	}
}

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route

func (s *Sentinel) NewRouter() *mux.Router {
	var routes = Routes{
		Route{
			"Index",
			"PUT",
			"/config/{name}",
			s.updateConfigHandler,
		},
	}
	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		router.Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}

	return router
}
