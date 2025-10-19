// Package tool provides tool implementations for the agent.
package tool

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strconv"

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

// Ensure RandomNumberTool implements TypedTool[RandomNumberParams]
var _ TypedTool[RandomNumberParams] = (*RandomNumberTool)(nil)

type RandomNumberTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *RandomNumberTool) Call(params RandomNumberParams) (string, error) {
	// Validate parameters
	if params.Min > params.Max {
		return "", ErrMinGreaterThanMax
	}

	// Generate random number
	rangeSize := params.Max - params.Min + 1
	n, err := rand.Int(rand.Reader, big.NewInt(int64(rangeSize)))
	if err != nil {
		return "", fmt.Errorf("generating random number: %w", err)
	}

	result := int(n.Int64()) + params.Min
	return strconv.Itoa(result), nil
}

func (t *RandomNumberTool) Param() anthropic.ToolParam {
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
