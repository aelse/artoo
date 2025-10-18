package tool

import "github.com/anthropics/anthropic-sdk-go"

type Tool interface {
	Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion
	Param() anthropic.ToolParam
}

var AllTools = []Tool{
	&RandomNumberTool{},
}
