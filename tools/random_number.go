// Package tools provides tool implementations for the agent.
package tools

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/anthropics/anthropic-sdk-go"
)

var (
	// ErrMinGreaterThanMax is returned when min value is greater than max value.
	ErrMinGreaterThanMax = errors.New("min value cannot be greater than max value")
)

// RandomNumberParams defines the parameters for generating a random number.
type RandomNumberParams struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// RandomNumberResponse defines the response for a random number generation.
type RandomNumberResponse struct {
	Number int `json:"number"`
}

// GenerateRandomNumber generates a random number between min and max values (inclusive).
func GenerateRandomNumber(params RandomNumberParams) (*RandomNumberResponse, error) {
	if params.Min > params.Max {
		return nil, ErrMinGreaterThanMax
	}

	// Use crypto/rand for secure random number generation.
	rangeSize := params.Max - params.Min + 1
	n, err := rand.Int(rand.Reader, big.NewInt(int64(rangeSize)))

	if err != nil {
		return nil, fmt.Errorf("generating random number: %w", err)
	}

	return &RandomNumberResponse{Number: int(n.Int64()) + params.Min}, nil
}

// Definition returns the tool definition for the random number generator.
func RandomNumberToolDefinition() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        "generate_random_number",
		Description: anthropic.String("Generate a random number between min and max values (inclusive)"),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"min": map[string]interface{}{
					"type":        "integer",
					"description": "Minimum value (inclusive)",
				},
				"max": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum value (inclusive)",
				},
			},
			Required: []string{"min", "max"},
		},
	}
}
