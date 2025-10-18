// Package tool provides tool implementations for the agent.
package tool

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
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

// RandomNumberResponse defines the response for a random number generation.
type RandomNumberResponse struct {
	Number int `json:"number"`
}

var _ Tool = (*RandomNumberTool)(nil)

type RandomNumberTool struct{}

func (t *RandomNumberTool) Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion {
	var params RandomNumberParams

	err := json.Unmarshal([]byte(block.JSON.Input.Raw()), &params)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stdout, "error unmarshalling params: %v\n", err)

		return nil
	}

	randomNumResp, err := t.generateRandomNumber(params)
	if err != nil {
		result := anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error: %v", err), true)

		return &result
	}

	_, _ = fmt.Fprintf(os.Stdout, "[Generated random number: %d]\n", randomNumResp.Number)

	result := anthropic.NewToolResultBlock(block.ID, strconv.Itoa(randomNumResp.Number), false)

	b, err := json.Marshal(randomNumResp)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, "error marshalling tool response: "+err.Error())

		return &result
	}

	_, _ = fmt.Fprintln(os.Stdout, string(b))

	return &result
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

// generateRandomNumber generates a random number between min and max values (inclusive).
func (t *RandomNumberTool) generateRandomNumber(params RandomNumberParams) (*RandomNumberResponse, error) {
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
