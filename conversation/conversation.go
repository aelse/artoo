// Package conversation provides conversation management functionality.
package conversation

import "github.com/anthropics/anthropic-sdk-go"

// Conversation manages the message history for an agent conversation.
type Conversation struct {
	messages []anthropic.MessageParam
}

// New creates a new empty Conversation.
func New() *Conversation {
	return &Conversation{
		messages: make([]anthropic.MessageParam, 0),
	}
}

// Append adds a message parameter to the conversation.
func (c *Conversation) Append(message anthropic.MessageParam) {
	c.messages = append(c.messages, message)
}

// Messages returns the slice of messages for use with the Claude API.
func (c *Conversation) Messages() []anthropic.MessageParam {
	return c.messages
}

// Len returns the number of messages in the conversation.
func (c *Conversation) Len() int {
	return len(c.messages)
}

// Get returns the message at the specified index.
func (c *Conversation) Get(index int) anthropic.MessageParam {
	return c.messages[index]
}
