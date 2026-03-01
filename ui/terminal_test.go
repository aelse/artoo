package ui

import (
	"sync"
	"testing"
)

func TestTerminal_ConcurrentOnToolResult(t *testing.T) {
	t.Parallel()

	term := NewTerminal()

	const numGoroutines = 20
	var wg sync.WaitGroup

	// Launch multiple goroutines all calling output methods simultaneously
	// These are the methods that can be called concurrently from the agent
	for range numGoroutines {
		wg.Go(func() {
			// Call output methods that will be called concurrently during tool execution
			term.OnText("test text")
			term.OnToolCall("testTool", `{"param": "value"}`)
			term.OnToolResult("testTool", "output", false)
			term.OnToolResult("testTool", "error", true)
		})
	}

	wg.Wait()
	// If we reach here without a panic or race condition, the test passes
}
