package tool

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// TypedTool is a generic interface for tools with strongly-typed parameters
type TypedTool[P any] interface {
	// Call executes the tool with typed parameters
	Call(params P) (string, error)

	// Param returns the tool definition for the Claude API
	Param() anthropic.ToolParam
}

// Tool is the non-generic interface that wraps TypedTool for use in collections
type Tool interface {
	Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion
	Param() anthropic.ToolParam
}

// toolWrapper wraps a TypedTool to implement the Tool interface
type toolWrapper[P any] struct {
	typed TypedTool[P]
}

// WrapTypedTool wraps a TypedTool into a Tool for registration
func WrapTypedTool[P any](t TypedTool[P]) Tool {
	return &toolWrapper[P]{typed: t}
}

// Call implements Tool.Call by unmarshalling and delegating to the typed tool
func (w *toolWrapper[P]) Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion {
	var params P

	// Unmarshal JSON into the typed params
	err := json.Unmarshal([]byte(block.JSON.Input.Raw()), &params)
	if err != nil {
		errMsg := fmt.Sprintf("Error unmarshalling parameters: %v", err)
		result := anthropic.NewToolResultBlock(block.ID, errMsg, true)
		return &result
	}

	// Call the typed tool with unmarshalled params
	output, err := w.typed.Call(params)
	if err != nil {
		errMsg := fmt.Sprintf("Error: %v", err)
		result := anthropic.NewToolResultBlock(block.ID, errMsg, true)
		return &result
	}

	// Return successful result
	result := anthropic.NewToolResultBlock(block.ID, output, false)
	return &result
}

// Param implements Tool.Param
func (w *toolWrapper[P]) Param() anthropic.ToolParam {
	return w.typed.Param()
}

var AllTools = []Tool{
	WrapTypedTool(&RandomNumberTool{}),
	WrapTypedTool(&GrepTool{}),
}
