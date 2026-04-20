// Copyright 2026 PgVillage
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

// Package logging brings a generic logging interface
package logging

import (
	"strings"

	"github.com/rs/zerolog"
)

// Components is a map that holds components and their Debug state (false is InfoLevel, True is DebugLevel)
type Components map[Component]zerolog.Level

// Component is a custom type, so that we can use it as an ENUM
type Component int

const (
	// KeeperComponent is the main component specifically for the Keeper commands
	KeeperComponent Component = iota
	// ProxyComponent is the main component specifically for the Proxy commands
	ProxyComponent Component = iota
	// SentinelComponent is the main component specifically for the Sentinel commands
	SentinelComponent Component = iota
	// CmdComponent is the main component specifically for stolon-cmd commands
	CmdComponent Component = iota

	// PgComponent is the component for all PostgreSQL logging
	PgComponent Component = iota
	// PgUtilsComponent is the component for all PostgreSQL logging
	PgUtilsComponent Component = iota

	// StoreComponent is the logging component for all code dealing with KV stores
	StoreComponent Component = iota

	// UnknownComponent represents a logging component with unknown origin
	UnknownComponent Component = iota
	// TestComponent represents a logging component only used in unittests
	TestComponent Component = iota
)

var (
	componentConverter = map[string]Component{
		"keeper":              KeeperComponent,
		"proxy":               ProxyComponent,
		"sentinel":            SentinelComponent,
		"stolon-cmd":          CmdComponent,
		"postgres":            PgComponent,
		"postgres-utils":      PgUtilsComponent,
		"kv-store":            StoreComponent,
		"undefined_component": UnknownComponent,
		"unittest_component":  TestComponent,
	}
	reverseComponentMap map[Component]string
)

func componentToString(component Component) string {
	if reverseComponentMap == nil {
		reverseComponentMap = map[Component]string{}
		for s, comp := range componentConverter {
			reverseComponentMap[comp] = s
		}
	}
	if s, exists := reverseComponentMap[component]; exists {
		return s
	}
	return "undefined_component"
}

// DebugComponentsFromString takes a comma separated string (as used in a command argument)
// and returns a Components object with all items set to debug
func DebugComponentsFromString(commaSeparated string) Components {
	components := Components{}
	for _, compName := range strings.Split(commaSeparated, ",") {
		if component, exists := componentConverter[compName]; exists {
			components[component] = zerolog.DebugLevel
		} else {
			components[UnknownComponent] = zerolog.DebugLevel
		}
	}
	return components
}

// NewComponentsFromStringMap takes a string map and converts it into a Components object
// where every component is set to the level represented by the string
func NewComponentsFromStringMap(enabledComponents map[string]string) Components {
	components := Components{}
	for compName, sLevel := range enabledComponents {
		level, ok := sToLevel[sLevel]
		if !ok {
			level = zerolog.DebugLevel
		}
		if component, exists := componentConverter[compName]; exists {
			components[component] = level
		} else {
			components[UnknownComponent] = level
		}
	}
	return components
}
