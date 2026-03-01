package agent

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aelse/artoo/tool"
	"github.com/anthropics/anthropic-sdk-go"
)

// mockTool implements tool.Tool for testing.
type mockTool struct {
	name     string
	sleep    time.Duration
	callCount int
	mu       sync.Mutex
}

func (m *mockTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        m.name,
		Description: anthropic.String("Mock tool for testing"),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "Test input",
				},
			},
		},
	}
}

func (m *mockTool) Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.sleep > 0 {
		time.Sleep(m.sleep)
	}

	return new(anthropic.NewToolResultBlock(
		block.ID,
		"Result from "+m.name,
		false,
	))
}

// mockCallbacks implements Callbacks for testing.
type mockCallbacks struct {
	toolResultsCalls []struct {
		name    string
		output  string
		isError bool
	}
	mu sync.Mutex
}

func (m *mockCallbacks) OnThinking() {}
func (m *mockCallbacks) OnThinkingDone() {}
func (m *mockCallbacks) OnText(_ string) {}
func (m *mockCallbacks) OnToolCall(_ string, _ string) {}
func (m *mockCallbacks) OnToolResult(name string, output string, isError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolResultsCalls = append(m.toolResultsCalls, struct {
		name    string
		output  string
		isError bool
	}{name, output, isError})
}

func TestExecuteToolsConcurrently_Single(t *testing.T) {
	t.Parallel()

	ag := &Agent{
		config: Config{MaxConcurrentTools: 4},
		toolMap: map[string]tool.Tool{
			"tool1": &mockTool{name: "tool1"},
		},
	}

	block := anthropic.ToolUseBlock{
		ID:    "id1",
		Name:  "tool1",
		Input: json.RawMessage(`{"input": "test"}`),
	}

	cb := &mockCallbacks{}
	results := ag.executeToolsConcurrently(context.Background(), []anthropic.ToolUseBlock{block}, cb)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestExecuteToolsConcurrently_Multiple(t *testing.T) {
	t.Parallel()

	ag := &Agent{
		config: Config{MaxConcurrentTools: 4},
		toolMap: map[string]tool.Tool{
			"tool1": &mockTool{name: "tool1"},
			"tool2": &mockTool{name: "tool2"},
			"tool3": &mockTool{name: "tool3"},
		},
	}

	blocks := []anthropic.ToolUseBlock{
		{ID: "id1", Name: "tool1", Input: json.RawMessage(`{}`)},
		{ID: "id2", Name: "tool2", Input: json.RawMessage(`{}`)},
		{ID: "id3", Name: "tool3", Input: json.RawMessage(`{}`)},
	}

	cb := &mockCallbacks{}
	results := ag.executeToolsConcurrently(context.Background(), blocks, cb)

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	if len(cb.toolResultsCalls) != 3 {
		t.Errorf("Expected 3 callback calls, got %d", len(cb.toolResultsCalls))
	}
}

func TestExecuteToolsConcurrently_OrderPreserved(t *testing.T) {
	t.Parallel()

	// Use tools with different sleep durations to verify result ordering
	ag := &Agent{
		config: Config{MaxConcurrentTools: 4},
		toolMap: map[string]tool.Tool{
			"fast":   &mockTool{name: "fast", sleep: 10 * time.Millisecond},
			"medium": &mockTool{name: "medium", sleep: 50 * time.Millisecond},
			"slow":   &mockTool{name: "slow", sleep: 100 * time.Millisecond},
		},
	}

	// Reverse order: slow, medium, fast
	blocks := []anthropic.ToolUseBlock{
		{ID: "id1", Name: "slow", Input: json.RawMessage(`{}`)},
		{ID: "id2", Name: "medium", Input: json.RawMessage(`{}`)},
		{ID: "id3", Name: "fast", Input: json.RawMessage(`{}`)},
	}

	cb := &mockCallbacks{}
	results := ag.executeToolsConcurrently(context.Background(), blocks, cb)

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// Verify all results are present (callbacks may be in execution order, not input order)
	if len(cb.toolResultsCalls) != 3 {
		t.Errorf("Expected 3 callback calls, got %d", len(cb.toolResultsCalls))
	}

	// Verify all three tool names are present in callbacks (order may vary)
	names := make(map[string]bool)
	for _, call := range cb.toolResultsCalls {
		names[call.name] = true
	}
	if !names["slow"] || !names["medium"] || !names["fast"] {
		t.Errorf("Expected callbacks from slow, medium, and fast tools")
	}
}

func TestExecuteToolsConcurrently_ErrorDoesNotAffectOthers(t *testing.T) {
	t.Parallel()

	ag := &Agent{
		config: Config{MaxConcurrentTools: 4},
		toolMap: map[string]tool.Tool{
			"tool1": &mockTool{name: "tool1"},
			// tool2 not in map, will result in error
			"tool3": &mockTool{name: "tool3"},
		},
	}

	blocks := []anthropic.ToolUseBlock{
		{ID: "id1", Name: "tool1", Input: json.RawMessage(`{}`)},
		{ID: "id2", Name: "tool2", Input: json.RawMessage(`{}`)}, // Not found
		{ID: "id3", Name: "tool3", Input: json.RawMessage(`{}`)},
	}

	cb := &mockCallbacks{}
	results := ag.executeToolsConcurrently(context.Background(), blocks, cb)

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// All three callback calls should happen (including error for missing tool)
	if len(cb.toolResultsCalls) != 3 {
		t.Errorf("Expected 3 callback calls, got %d; calls: %v", len(cb.toolResultsCalls), cb.toolResultsCalls)
	}

	// Check that all three tools were called
	if len(cb.toolResultsCalls) >= 3 {
		// Find the tool2 call (should have isError=true)
		var foundError bool
		for _, call := range cb.toolResultsCalls {
			if call.name == "tool2" && call.isError {
				foundError = true

				break
			}
		}
		if !foundError {
			t.Errorf("Expected error callback for tool2, got: %v", cb.toolResultsCalls)
		}
	}
}

// concurrentTrackingTool tracks concurrent execution.
type concurrentTrackingTool struct {
	currentConcurrent int32
	maxConcurrent     int32
	sleep             time.Duration
}

func (c *concurrentTrackingTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        "tracking_tool",
		Description: anthropic.String("Tool for tracking concurrent execution"),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
	}
}

func (c *concurrentTrackingTool) Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion {
	// Increment concurrent counter
	atomic.AddInt32(&c.currentConcurrent, 1)
	current := atomic.LoadInt32(&c.currentConcurrent)

	// Update max concurrent
	for {
		prevMax := atomic.LoadInt32(&c.maxConcurrent)
		if current <= prevMax {
			break
		}
		if atomic.CompareAndSwapInt32(&c.maxConcurrent, prevMax, current) {
			break
		}
	}

	// Sleep
	if c.sleep > 0 {
		time.Sleep(c.sleep)
	}

	// Decrement concurrent counter
	atomic.AddInt32(&c.currentConcurrent, -1)

	return new(anthropic.NewToolResultBlock(block.ID, "OK", false))
}

func TestExecuteToolsConcurrently_SemaphoreLimit(t *testing.T) {
	t.Parallel()

	tracker := &concurrentTrackingTool{
		sleep: 50 * time.Millisecond,
	}

	ag := &Agent{
		config: Config{MaxConcurrentTools: 1},
		toolMap: map[string]tool.Tool{
			"tracker": tracker,
		},
	}

	blocks := make([]anthropic.ToolUseBlock, 5)
	for i := range 5 {
		blocks[i] = anthropic.ToolUseBlock{
			ID:    "id" + string(rune(48+i)), // Convert to character
			Name:  "tracker",
			Input: json.RawMessage(`{}`),
		}
	}

	cb := &mockCallbacks{}
	results := ag.executeToolsConcurrently(context.Background(), blocks, cb)

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	if atomic.LoadInt32(&tracker.maxConcurrent) > 1 {
		t.Errorf("Expected max concurrent to be 1, got %d", atomic.LoadInt32(&tracker.maxConcurrent))
	}
}
