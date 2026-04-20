package logging

import (
	"bytes"
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

const (
	defaultLevelString = "info"
	enabledLevelString = "debug"
)

type logSink struct {
	logs []string
	buf  bytes.Buffer
}

func (l *logSink) Write(p []byte) (n int, err error) {
	l.logs = append(l.logs, string(p))
	return l.buf.Write(p)
}

func (l *logSink) Index(i int) string {
	if i >= 0 && i < len(l.logs) {
		return l.logs[i]
	}
	return ""
}

func (l *logSink) String() string {
	return l.buf.String()
}

func (l *logSink) Reset() {
	l.logs = []string{}
	l.buf.Reset()
}

func TestDebuggingStatic(t *testing.T) {
	const comp1 = TestComponent
	SetDynamicLoggingConfig(defaultLevelString, nil)
	ctx := context.TODO()
	// debug false
	SetStaticLevel(defaultLevelString)
	_, noDebugLogger := GetLogComponent(ctx, comp1)
	assert.Equal(t, zerolog.InfoLevel, noDebugLogger.GetLevel())
	// debug true
	SetStaticLevel(enabledLevelString)
	_, allDebugLogger := GetLogComponent(ctx, comp1)
	assert.Equal(t, zerolog.DebugLevel, allDebugLogger.GetLevel())
	// debug component
	SetStaticComponents(Components{comp1: zerolog.DebugLevel})
	_, componentDebugLogger := GetLogComponent(ctx, comp1)
	assert.Equal(t, zerolog.DebugLevel, componentDebugLogger.GetLevel())
}

func TestDebuggingDynamic(t *testing.T) {
	ctx := context.TODO()
	const comp1 = TestComponent
	SetStaticLevel(defaultLevelString)
	SetStaticComponents(nil)
	// debug false
	SetDynamicLoggingConfig(defaultLevelString, nil)
	_, noDebugLogger := GetLogComponent(ctx, comp1)
	assert.Equal(t, zerolog.InfoLevel, noDebugLogger.GetLevel())
	// debug true
	SetDynamicLoggingConfig(enabledLevelString, nil)
	_, allDebugLogger := GetLogComponent(ctx, comp1)
	assert.Equal(t, zerolog.DebugLevel, allDebugLogger.GetLevel())
	// debug component on
	SetDynamicLoggingConfig(defaultLevelString, Components{comp1: zerolog.DebugLevel})
	_, componentDebugLogger := GetLogComponent(ctx, comp1)
	assert.Equal(t, zerolog.DebugLevel, componentDebugLogger.GetLevel())

	// debug component off
	SetDynamicLoggingConfig(defaultLevelString, Components{})
	_, componentNoDebugLogger := GetLogComponent(ctx, comp1)
	assert.Equal(t, zerolog.InfoLevel, componentNoDebugLogger.GetLevel())
}

func TestOutput(t *testing.T) {
	// Keep original output
	originalOut := output.Out
	sink := &logSink{}
	output.Out = sink
	// Restore original output after test
	defer func() {
		output.Out = originalOut
	}()

	ctx := context.TODO()
	const comp = TestComponent

	// Test Info level
	SetStaticLevel("info")
	SetStaticComponents(nil)
	SetDynamicLoggingConfig("info", nil)
	_, logger := GetLogComponent(ctx, comp)

	logger.Debug().Msg("this is a debug message")
	assert.Empty(t, sink.String(), "Debug message should not be logged at info level")
	sink.Reset()

	logger.Info().Msg("this is an info message")
	assert.Contains(t, sink.String(), "this is an info message", "Info message should be logged at info level")
	sink.Reset()

	logger.Warn().Msg("this is a warning message")
	assert.Contains(t, sink.String(), "this is a warning message", "Warning message should be logged at info level")
	sink.Reset()

	// Test Error level
	SetStaticLevel("error")
	SetDynamicLoggingConfig("error", nil)
	_, logger = GetLogComponent(ctx, comp)
	logger.Warn().Msg("this is another warning message")
	assert.Empty(t, sink.String(), "Warning message should not be logged at error level")
	sink.Reset()

	logger.Error().Msg("this is an error message")
	assert.Contains(t, sink.String(), "this is an error message", "Error message should be logged at error level")
	sink.Reset()
}

func TestInvalidLevel(t *testing.T) {
	// Test that setting an invalid level falls back to the default (info)
	const comp = TestComponent
	ctx := context.TODO()
	SetStaticLevel("invalidlevel")
	SetStaticComponents(nil)
	SetDynamicLoggingConfig("anotherinvalidlevel", nil)

	_, logger := GetLogComponent(ctx, comp)

	assert.Equal(t, zerolog.InfoLevel, logger.GetLevel(), "Logger level should default to Info for invalid level strings")
}

func TestColoring(t *testing.T) {
	originalNoColor := output.NoColor

	EnableColor()
	assert.False(t, output.NoColor, "EnableColor should set NoColor to false")

	DisableColor()
	assert.True(t, output.NoColor, "DisableColor should set NoColor to true")

	// Restore original
	output.NoColor = originalNoColor
}
