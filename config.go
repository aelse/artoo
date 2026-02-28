// Package main provides configuration loading from environment variables.
package main

import (
	"os"
	"strconv"

	"github.com/aelse/artoo/agent"
	"github.com/aelse/artoo/conversation"
)

// AppConfig holds all configuration for the artoo application,
// loaded from environment variables with sensible defaults.
type AppConfig struct {
	Agent        agent.Config
	Conversation conversation.Config
	Debug        bool
}

// LoadConfig loads configuration from environment variables.
// Unset variables use sensible defaults.
func LoadConfig() AppConfig {
	return AppConfig{
		Agent: agent.Config{
			Model:     getEnv("ARTOO_MODEL", "claude-sonnet-4-20250514"),
			MaxTokens: getEnvInt64("ARTOO_MAX_TOKENS", 8192),
		},
		Conversation: conversation.Config{
			MaxContextTokens:   getEnvInt("ARTOO_MAX_CONTEXT_TOKENS", 180_000),
			ToolResultMaxChars: getEnvInt("ARTOO_TOOL_RESULT_MAX_CHARS", 10_000),
		},
		Debug: getEnvBool("ARTOO_DEBUG", false),
	}
}

// getEnv returns the value of the environment variable key, or defaultValue if not set.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvInt returns the integer value of the environment variable key,
// or defaultValue if not set or invalid. Invalid values are logged and default is used.
func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvInt64 returns the int64 value of the environment variable key,
// or defaultValue if not set or invalid.
func getEnvInt64(key string, defaultValue int64) int64 {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvBool returns the boolean value of the environment variable key,
// or defaultValue if not set or invalid.
// Valid true values: "1", "true", "yes", "on" (case-insensitive)
// Valid false values: "0", "false", "no", "off" (case-insensitive)
func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		switch value {
		case "1", "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
			return true
		case "0", "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF":
			return false
		}
	}
	return defaultValue
}
