package logging

import (
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentToString(t *testing.T) {
	var nonexistentComponent Component = -1
	assert.Equal(t, componentToString(nonexistentComponent), componentToString(UnknownComponent))
}

func TestNewComponentsFromString(t *testing.T) {
	var (
		components = []string{
			"keeper",
			"proxy",
			"undefined_component",
			"unittest_component",
		}
		commaSeparatedString = strings.Join(components, ",")
		result               = DebugComponentsFromString(commaSeparatedString)
	)
	for _, compName := range components {
		comp, exists := componentConverter[compName]
		if !exists {
			comp = UnknownComponent
		}
		require.Contains(t, result, comp)
		assert.Equal(t, zerolog.DebugLevel, result[comp])
	}
}

func TestNewComponentsFromStringMap(t *testing.T) {
	var (
		debugComponents = []string{
			"keeper",
			"proxy",
		}
		warnComponents = []string{
			"sentinel",
			"stolon-cmd",
		}
		strMap = map[string]string{}
	)
	for _, compName := range debugComponents {
		strMap[compName] = "debug"
	}
	for _, compName := range warnComponents {
		strMap[compName] = "warn"
	}
	result := NewComponentsFromStringMap(strMap)

	for _, compName := range debugComponents {
		comp, exists := componentConverter[compName]
		require.True(t, exists)
		level, exists := result[comp]
		require.True(t, exists)
		require.Equal(t, level, zerolog.DebugLevel)
	}
	for _, compName := range warnComponents {
		comp, exists := componentConverter[compName]
		require.True(t, exists)
		level, exists := result[comp]
		require.True(t, exists)
		require.Equal(t, level, zerolog.WarnLevel)
	}
}
