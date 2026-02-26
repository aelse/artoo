// Package agent provides the core agent functionality for interacting with Claude.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aelse/artoo/conversation"
	"github.com/aelse/artoo/tool"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Agent struct {
	client          anthropic.Client
	conversation    *conversation.Conversation
	tools           []tool.Tool
	toolMap         map[string]tool.Tool
	toolUnionParams []anthropic.ToolUnionParam
}

const (
	maxTokens           = 1024
	spinnerTickInterval = 100 * time.Millisecond
)

var (
	titleStyle  lipgloss.Style
	userStyle   lipgloss.Style
	claudeStyle lipgloss.Style
	debugStyle  lipgloss.Style
	errorStyle  lipgloss.Style
	promptStyle lipgloss.Style
)

func init() {
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true) // Bright cyan
	userStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))            // Magenta
	claudeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))           // Blue
	debugStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // Grey
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)  // Red
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))          // Magenta
}

// spinnerRunner manages a simple terminal spinner.
type spinnerRunner struct {
	model   spinner.Model
	message string
	quit    chan bool
	done    sync.WaitGroup
}

// newSpinner creates a new spinner with the given message.
func newSpinner(message string) *spinnerRunner {
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = promptStyle

	return &spinnerRunner{
		model:   s,
		message: message,
		quit:    make(chan bool),
	}
}

// start begins the spinner animation.
func (s *spinnerRunner) start() {
	s.done.Add(1)

	go func() {
		defer s.done.Done()

		ticker := time.NewTicker(spinnerTickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-s.quit:
				// Clear the spinner line.
				_, _ = fmt.Fprint(os.Stdout, "\r\033[K")

				return
			case <-ticker.C:
				s.model, _ = s.model.Update(s.model.Tick())
				frame := s.model.View()
				_, _ = fmt.Fprintf(os.Stdout, "\r%s %s", frame, s.message)
			}
		}
	}()
}

// stop ends the spinner animation and clears the line.
func (s *spinnerRunner) stop() {
	close(s.quit)
	s.done.Wait()
}

// inputModel is the Bubble Tea model for text input.
type inputModel struct {
	textInput textinput.Model
	submitted bool
	value     string
}

// newInputModel creates a new input model.
func newInputModel() inputModel {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Focus()
	ti.Prompt = userStyle.Render("> ")
	ti.PromptStyle = lipgloss.NewStyle()
	ti.TextStyle = lipgloss.NewStyle()
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return inputModel{
		textInput: ti,
		submitted: false,
	}
}

// Init initializes the input model.
func (m inputModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles input events.
func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			m.value = m.textInput.Value()
			m.submitted = true

			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			m.value = ""
			m.submitted = true

			return m, tea.Quit
		default:
			// Let textinput handle other keys.
			m.textInput, cmd = m.textInput.Update(msg)

			return m, cmd
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)

	return m, cmd
}

// View renders the input model.
func (m inputModel) View() string {
	return m.textInput.View()
}

func New(client anthropic.Client) *Agent {
	allTools := tool.AllTools
	return &Agent{
		client:          client,
		conversation:    conversation.New(),
		tools:           allTools,
		toolMap:         makeToolMap(allTools),
		toolUnionParams: makeToolUnionParams(allTools),
	}
}

func (a *Agent) Run(ctx context.Context) error {

	_, _ = fmt.Fprintln(os.Stdout, titleStyle.Render("Artoo Agent")+" - Type 'quit' to exit")

	readyForUserInput := true

	for {
		var userInput string

		if readyForUserInput {
			userInput = a.getUserInput()
			if userInput == "" {
				break
			}

			if userInput == "quit" || userInput == "exit" {
				break
			}

			a.conversation.Append(anthropic.NewUserMessage(
				anthropic.NewTextBlock(userInput),
			))
			readyForUserInput = false
		}

		a.printConversation()

		message, err := a.callClaude(ctx, a.toolUnionParams)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stdout, "%s\n", errorStyle.Render(fmt.Sprintf("Error: %v", err)))

			continue
		}

		toolResults, hasToolUse := a.processResponse(message)

		if len(toolResults) > 0 {
			a.conversation.Append(anthropic.NewUserMessage(toolResults...))
		}

		if !hasToolUse {
			readyForUserInput = true
		}

		_, _ = fmt.Fprint(os.Stdout, "\n\n")
	}

	return nil
}

func makeToolUnionParams(tools []tool.Tool) []anthropic.ToolUnionParam {
	tup := make([]anthropic.ToolUnionParam, len(tools))
	for i := range tools {
		toolParam := tools[i].Param()
		tup[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
	}
	return tup
}

func makeToolMap(tools []tool.Tool) map[string]tool.Tool {
	toolMap := make(map[string]tool.Tool)
	for i := range tools {
		t := tools[i]
		toolMap[t.Param().Name] = tools[i]
	}
	return toolMap
}

func (a *Agent) getUserInput() string {
	m := newInputModel()
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return ""
	}

	if im, ok := finalModel.(inputModel); ok {
		return strings.TrimSpace(im.value)
	}

	return ""
}

func (a *Agent) printConversation() {
	fmt.Println(debugStyle.Render("Calling claude with conversation:"))

	for i := 0; i < a.conversation.Len(); i++ {
		m, err := json.Marshal(a.conversation.Get(i))
		if err != nil {
			fmt.Printf("%s", errorStyle.Render(fmt.Sprintf("[%d] error marshalling: %v\n", i, err)))
			continue
		}

		fmt.Println(debugStyle.Render(fmt.Sprintf("[%d] %s", i, string(m))))
	}
}

func (a *Agent) callClaude(ctx context.Context, tools []anthropic.ToolUnionParam) (*anthropic.Message, error) {
	spin := newSpinner("Thinking...")
	spin.start()
	defer spin.stop()

	message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude4Sonnet20250514,
		MaxTokens: maxTokens,
		Messages:  a.conversation.Messages(),
		Tools:     tools,
	})

	return message, err
}

func (a *Agent) processResponse(message *anthropic.Message) ([]anthropic.ContentBlockParamUnion, bool) {
	var toolResults []anthropic.ContentBlockParamUnion

	hasToolUse := false

	_, _ = fmt.Fprint(os.Stdout, claudeStyle.Render("Claude")+": ")

	a.printMessageContent(message)
	a.conversation.Append(message.ToParam())

	for _, block := range message.Content {
		variant, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}

		hasToolUse = true

		result := a.handleToolUse(variant)
		if result != nil {
			toolResults = append(toolResults, *result)
		}
	}

	return toolResults, hasToolUse
}

func (a *Agent) printMessageContent(message *anthropic.Message) {
	for _, block := range message.Content {
		switch block := block.AsAny().(type) {
		case anthropic.TextBlock:
			_, _ = fmt.Fprintln(os.Stdout, "text: "+block.Text)
		case anthropic.ToolUseBlock:
			inputJSON, err := json.Marshal(block.Input)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stdout, "error marshalling input: "+err.Error())

				continue
			}

			_, _ = fmt.Fprintln(os.Stdout, block.Name+": "+string(inputJSON))
		}
	}
}

func (a *Agent) handleToolUse(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion {
	spin := newSpinner("Calling tool...")
	spin.start()
	defer spin.stop()

	t, exists := a.toolMap[block.Name]
	if !exists {
		return nil
	}

	return t.Call(block)
}

func Run(ctx context.Context) error {
	client := anthropic.NewClient()
	agent := New(client)

	return agent.Run(ctx)
}
