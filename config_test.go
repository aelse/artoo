package main

import (
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Don't set any ARTOO env vars, so defaults are used
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

	if cfg.Agent.MaxConcurrentTools != 4 {
		t.Errorf("default max concurrent tools should be 4, got %d", cfg.Agent.MaxConcurrentTools)
	}

	if cfg.Debug != false {
		t.Errorf("default debug should be false, got %v", cfg.Debug)
	}
}

func TestLoadConfig_FromEnv(t *testing.T) {
	t.Setenv("ARTOO_MODEL", "claude-opus-4-20250805")
	t.Setenv("ARTOO_MAX_TOKENS", "16384")
	t.Setenv("ARTOO_MAX_CONTEXT_TOKENS", "200000")
	t.Setenv("ARTOO_TOOL_RESULT_MAX_CHARS", "20000")
	t.Setenv("ARTOO_MAX_CONCURRENT_TOOLS", "8")
	t.Setenv("ARTOO_DEBUG", "true")

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

	if cfg.Agent.MaxConcurrentTools != 8 {
		t.Errorf("max concurrent tools from env should be 8, got %d", cfg.Agent.MaxConcurrentTools)
	}

	if cfg.Debug != true {
		t.Errorf("debug from env should be true, got %v", cfg.Debug)
	}
}

func TestGetEnv(t *testing.T) {
	// Test unset - use a variable name that shouldn't be set
	if getEnv("_ARTOO_NONEXISTENT_VAR", "default") != "default" {
		t.Error("getEnv should return default for unset var")
	}

	// Test set
	t.Setenv("TEST_VAR", "value")
	if getEnv("TEST_VAR", "default") != "value" {
		t.Error("getEnv should return env var value")
	}
}

func TestGetEnvInt(t *testing.T) {
	// Test unset - use a variable name that shouldn't be set
	if getEnvInt("_ARTOO_NONEXISTENT_INT", 42) != 42 {
		t.Error("getEnvInt should return default for unset var")
	}

	// Test valid
	t.Setenv("TEST_INT", "100")
	if getEnvInt("TEST_INT", 42) != 100 {
		t.Error("getEnvInt should parse valid int")
	}

	// Test invalid (should use default)
	t.Setenv("TEST_INT_INVALID", "not-a-number")
	if getEnvInt("TEST_INT_INVALID", 42) != 42 {
		t.Error("getEnvInt should return default for invalid int")
	}
}

func TestGetEnvInt64(t *testing.T) {
	// Test unset - use a variable name that shouldn't be set
	if getEnvInt64("_ARTOO_NONEXISTENT_INT64", 42) != 42 {
		t.Error("getEnvInt64 should return default for unset var")
	}

	// Test valid
	t.Setenv("TEST_INT64", "9999999999")
	if getEnvInt64("TEST_INT64", 42) != 9999999999 {
		t.Error("getEnvInt64 should parse valid int64")
	}

	// Test invalid (should use default)
	t.Setenv("TEST_INT64_INVALID", "not-a-number")
	if getEnvInt64("TEST_INT64_INVALID", 42) != 42 {
		t.Error("getEnvInt64 should return default for invalid int64")
	}
}

func TestGetEnvBool(t *testing.T) {
	// Test unset - use a variable name that shouldn't be set
	if getEnvBool("_ARTOO_NONEXISTENT_BOOL", false) != false {
		t.Error("getEnvBool should return default for unset var")
	}

	// Test true values
	trueValues := []string{"1", "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON"}
	for _, val := range trueValues {
		t.Setenv("TEST_BOOL_VAL", val)
		if !getEnvBool("TEST_BOOL_VAL", false) {
			t.Errorf("getEnvBool should parse %q as true", val)
		}
	}

	// Test false values
	falseValues := []string{"0", "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF"}
	for _, val := range falseValues {
		t.Setenv("TEST_BOOL_VAL", val)
		if getEnvBool("TEST_BOOL_VAL", true) {
			t.Errorf("getEnvBool should parse %q as false", val)
		}
	}

	// Test invalid (should use default)
	t.Setenv("TEST_BOOL_INVALID", "maybe")
	if getEnvBool("TEST_BOOL_INVALID", true) != true {
		t.Error("getEnvBool should return default for invalid value")
	}
}

func TestGetEnvBool_DefaultTrue(t *testing.T) {
	// Test unset with default true - use a variable name that shouldn't be set
	if getEnvBool("_ARTOO_NONEXISTENT_BOOL_TRUE", true) != true {
		t.Error("getEnvBool should return true default")
	}

	// Test set to false overrides default true
	t.Setenv("TEST_BOOL_OVERRIDE", "false")
	if getEnvBool("TEST_BOOL_OVERRIDE", true) != false {
		t.Error("getEnvBool should override default true with false")
	}
}

func TestPartialEnvConfig(t *testing.T) {
	// Set only model
	t.Setenv("ARTOO_MODEL", "custom-model")

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
