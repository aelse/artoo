package main

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear all artoo env vars to test defaults
	defer clearEnv(
		"ARTOO_MODEL",
		"ARTOO_MAX_TOKENS",
		"ARTOO_MAX_CONTEXT_TOKENS",
		"ARTOO_TOOL_RESULT_MAX_CHARS",
		"ARTOO_DEBUG",
	)

	cfg := LoadConfig()

	if cfg.Agent.Model != "claude-sonnet-4-20250514" {
		t.Errorf("default model should be claude-sonnet-4-20250514, got %s", cfg.Agent.Model)
	}

	if cfg.Agent.MaxTokens != 8192 {
		t.Errorf("default max tokens should be 8192, got %d", cfg.Agent.MaxTokens)
	}

	if cfg.Conversation.MaxContextTokens != 180_000 {
		t.Errorf("default max context tokens should be 180000, got %d", cfg.Conversation.MaxContextTokens)
	}

	if cfg.Conversation.ToolResultMaxChars != 10_000 {
		t.Errorf("default tool result max chars should be 10000, got %d", cfg.Conversation.ToolResultMaxChars)
	}

	if cfg.Debug != false {
		t.Errorf("default debug should be false, got %v", cfg.Debug)
	}
}

func TestLoadConfig_FromEnv(t *testing.T) {
	defer clearEnv(
		"ARTOO_MODEL",
		"ARTOO_MAX_TOKENS",
		"ARTOO_MAX_CONTEXT_TOKENS",
		"ARTOO_TOOL_RESULT_MAX_CHARS",
		"ARTOO_DEBUG",
	)

	os.Setenv("ARTOO_MODEL", "claude-opus-4-20250805")
	os.Setenv("ARTOO_MAX_TOKENS", "16384")
	os.Setenv("ARTOO_MAX_CONTEXT_TOKENS", "200000")
	os.Setenv("ARTOO_TOOL_RESULT_MAX_CHARS", "20000")
	os.Setenv("ARTOO_DEBUG", "true")

	cfg := LoadConfig()

	if cfg.Agent.Model != "claude-opus-4-20250805" {
		t.Errorf("model from env should be claude-opus-4-20250805, got %s", cfg.Agent.Model)
	}

	if cfg.Agent.MaxTokens != 16384 {
		t.Errorf("max tokens from env should be 16384, got %d", cfg.Agent.MaxTokens)
	}

	if cfg.Conversation.MaxContextTokens != 200_000 {
		t.Errorf("max context tokens from env should be 200000, got %d", cfg.Conversation.MaxContextTokens)
	}

	if cfg.Conversation.ToolResultMaxChars != 20_000 {
		t.Errorf("tool result max chars from env should be 20000, got %d", cfg.Conversation.ToolResultMaxChars)
	}

	if cfg.Debug != true {
		t.Errorf("debug from env should be true, got %v", cfg.Debug)
	}
}

func TestGetEnv(t *testing.T) {
	defer clearEnv("TEST_VAR")

	// Test unset
	if getEnv("TEST_VAR", "default") != "default" {
		t.Error("getEnv should return default for unset var")
	}

	// Test set
	os.Setenv("TEST_VAR", "value")
	if getEnv("TEST_VAR", "default") != "value" {
		t.Error("getEnv should return env var value")
	}
}

func TestGetEnvInt(t *testing.T) {
	defer clearEnv("TEST_INT")

	// Test unset
	if getEnvInt("TEST_INT", 42) != 42 {
		t.Error("getEnvInt should return default for unset var")
	}

	// Test valid
	os.Setenv("TEST_INT", "100")
	if getEnvInt("TEST_INT", 42) != 100 {
		t.Error("getEnvInt should parse valid int")
	}

	// Test invalid (should use default)
	os.Setenv("TEST_INT", "not-a-number")
	if getEnvInt("TEST_INT", 42) != 42 {
		t.Error("getEnvInt should return default for invalid int")
	}
}

func TestGetEnvInt64(t *testing.T) {
	defer clearEnv("TEST_INT64")

	// Test unset
	if getEnvInt64("TEST_INT64", 42) != 42 {
		t.Error("getEnvInt64 should return default for unset var")
	}

	// Test valid
	os.Setenv("TEST_INT64", "9999999999")
	if getEnvInt64("TEST_INT64", 42) != 9999999999 {
		t.Error("getEnvInt64 should parse valid int64")
	}

	// Test invalid (should use default)
	os.Setenv("TEST_INT64", "not-a-number")
	if getEnvInt64("TEST_INT64", 42) != 42 {
		t.Error("getEnvInt64 should return default for invalid int64")
	}
}

func TestGetEnvBool(t *testing.T) {
	defer clearEnv("TEST_BOOL")

	// Test unset
	if getEnvBool("TEST_BOOL", false) != false {
		t.Error("getEnvBool should return default for unset var")
	}

	// Test true values
	trueValues := []string{"1", "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON"}
	for _, val := range trueValues {
		os.Setenv("TEST_BOOL", val)
		if !getEnvBool("TEST_BOOL", false) {
			t.Errorf("getEnvBool should parse %q as true", val)
		}
	}

	// Test false values
	falseValues := []string{"0", "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF"}
	for _, val := range falseValues {
		os.Setenv("TEST_BOOL", val)
		if getEnvBool("TEST_BOOL", true) {
			t.Errorf("getEnvBool should parse %q as false", val)
		}
	}

	// Test invalid (should use default)
	os.Setenv("TEST_BOOL", "maybe")
	if getEnvBool("TEST_BOOL", true) != true {
		t.Error("getEnvBool should return default for invalid value")
	}
}

func TestGetEnvBool_DefaultTrue(t *testing.T) {
	defer clearEnv("TEST_BOOL")

	// Test unset with default true
	if getEnvBool("TEST_BOOL", true) != true {
		t.Error("getEnvBool should return true default")
	}

	// Test set to false overrides default true
	os.Setenv("TEST_BOOL", "false")
	if getEnvBool("TEST_BOOL", true) != false {
		t.Error("getEnvBool should override default true with false")
	}
}

func TestPartialEnvConfig(t *testing.T) {
	defer clearEnv(
		"ARTOO_MODEL",
		"ARTOO_MAX_TOKENS",
		"ARTOO_MAX_CONTEXT_TOKENS",
		"ARTOO_TOOL_RESULT_MAX_CHARS",
		"ARTOO_DEBUG",
	)

	// Set only model
	os.Setenv("ARTOO_MODEL", "custom-model")

	cfg := LoadConfig()

	if cfg.Agent.Model != "custom-model" {
		t.Error("should use custom model from env")
	}

	if cfg.Agent.MaxTokens != 8192 {
		t.Error("should use default max tokens")
	}

	if cfg.Conversation.MaxContextTokens != 180_000 {
		t.Error("should use default max context tokens")
	}

	if cfg.Debug != false {
		t.Error("should use default debug")
	}
}

// Helper to clear environment variables
func clearEnv(vars ...string) {
	for _, v := range vars {
		os.Unsetenv(v)
	}
}
