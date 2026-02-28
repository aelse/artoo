package conversation

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestNew(t *testing.T) {
	c := New()
	if c.MessageCount() != 0 {
		t.Errorf("new conversation should have 0 messages, got %d", c.MessageCount())
	}
	if c.EstimatedTokens() != 0 {
		t.Errorf("new conversation should have 0 tokens, got %d", c.EstimatedTokens())
	}
}

func TestAppend(t *testing.T) {
	c := New()
	msg := anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))
	c.Append(msg)

	if c.MessageCount() != 1 {
		t.Errorf("after append, expected 1 message, got %d", c.MessageCount())
	}

	// Just verify we can retrieve the message without error
	_ = c.Get(0)
}

func TestUpdateTokenCount(t *testing.T) {
	c := New()
	c.UpdateTokenCount(1000)

	if c.EstimatedTokens() != 1000 {
		t.Errorf("expected 1000 tokens, got %d", c.EstimatedTokens())
	}

	c.UpdateTokenCount(2000)
	if c.EstimatedTokens() != 2000 {
		t.Errorf("expected 2000 tokens after update, got %d", c.EstimatedTokens())
	}
}

func TestTrim_BelowThreshold(t *testing.T) {
	cfg := Config{
		MaxContextTokens:   100,
		ToolResultMaxChars: 1000,
	}
	c := NewWithConfig(cfg)

	// Add a few messages
	c.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("msg1")))
	c.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("msg2")))
	c.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("msg3")))

	initialCount := c.MessageCount()

	// Set token count below threshold (75 out of 100)
	c.UpdateTokenCount(50)
	c.Trim()

	// Should not trim since below threshold
	if c.MessageCount() != initialCount {
		t.Errorf("trim below threshold should not remove messages, was %d now %d",
			initialCount, c.MessageCount())
	}
}

func TestTrim_AboveThreshold(t *testing.T) {
	cfg := Config{
		MaxContextTokens:   100,
		ToolResultMaxChars: 1000,
	}
	c := NewWithConfig(cfg)

	// Add multiple messages
	for i := 0; i < 5; i++ {
		c.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("msg")))
	}

	initialCount := c.MessageCount()

	// Set token count above threshold (85 out of 100 = above 75% threshold)
	c.UpdateTokenCount(85)
	c.Trim()

	// Should trim since above 75% threshold
	if c.MessageCount() >= initialCount {
		t.Errorf("trim above threshold should remove messages, expected < %d, got %d",
			initialCount, c.MessageCount())
	}

	// Should keep at least system message + some recent context
	if c.MessageCount() < 2 {
		t.Errorf("trim should keep at least 2 messages for context, got %d", c.MessageCount())
	}
}

func TestTruncateToolResult_LargeOutput(t *testing.T) {
	cfg := Config{
		MaxContextTokens:   1000,
		ToolResultMaxChars: 100, // Small limit for testing
	}
	c := NewWithConfig(cfg)

	// Create a tool result with large text
	largeText := ""
	for i := 0; i < 50; i++ {
		largeText += "1234567890"
	}

	result := anthropic.NewToolResultBlock("tool-1", largeText, false)
	truncated := c.truncateToolResult(result)

	// Extract text from truncated result
	if truncated.OfToolResult == nil || len(truncated.OfToolResult.Content) == 0 {
		t.Fatal("truncated result should have tool result content")
	}

	text := truncated.OfToolResult.Content[0].OfText.Text

	// Should be truncated and contain truncation notice
	if len(text) > 200 { // Should be ~100 chars + truncation notice
		t.Errorf("truncated text is too long: %d chars", len(text))
	}

	if !contains(text, "truncated") {
		t.Error("truncated text should contain 'truncated' notice")
	}
}

func TestTruncateToolResult_SmallOutput(t *testing.T) {
	cfg := Config{
		MaxContextTokens:   1000,
		ToolResultMaxChars: 1000,
	}
	c := NewWithConfig(cfg)

	result := anthropic.NewToolResultBlock("tool-1", "small output", false)
	truncated := c.truncateToolResult(result)

	if truncated.OfToolResult == nil {
		t.Fatal("truncated result should still be a tool result")
	}

	text := truncated.OfToolResult.Content[0].OfText.Text
	if text != "small output" {
		t.Errorf("small output should not be changed, got %q", text)
	}
}

func TestMessageCount(t *testing.T) {
	c := New()

	for i := 0; i < 5; i++ {
		c.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("msg")))
		if c.MessageCount() != i+1 {
			t.Errorf("after %d appends, expected count %d, got %d", i+1, i+1, c.MessageCount())
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxContextTokens <= 0 {
		t.Error("default MaxContextTokens should be positive")
	}

	if cfg.ToolResultMaxChars <= 0 {
		t.Error("default ToolResultMaxChars should be positive")
	}

	if cfg.MaxContextTokens < 100_000 {
		t.Error("default MaxContextTokens should be reasonable for Sonnet model")
	}
}

func TestNoTrimWithZeroLimit(t *testing.T) {
	cfg := Config{
		MaxContextTokens:   0, // No limit
		ToolResultMaxChars: 1000,
	}
	c := NewWithConfig(cfg)

	c.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("msg")))
	c.UpdateTokenCount(999999) // Extremely high

	c.Trim() // Should do nothing

	if c.MessageCount() != 1 {
		t.Error("with MaxContextTokens=0, trim should not remove messages")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
