// Package ui provides terminal user interface components.
package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aelse/artoo/agent"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Ensure Terminal implements agent.Callbacks.
var _ agent.Callbacks = (*Terminal)(nil)

// Style definitions.
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

const spinnerTickInterval = 100 * time.Millisecond

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
	s.done.Go(func() {
		ticker := time.Tick(spinnerTickInterval)

		for {
			select {
			case <-s.quit:
				// Clear the spinner line.
				_, _ = fmt.Fprint(os.Stdout, "\r\033[K")

				return
			case <-ticker:
				s.model, _ = s.model.Update(s.model.Tick())
				frame := s.model.View()
				_, _ = fmt.Fprintf(os.Stdout, "\r%s %s", frame, s.message)
			}
		}
	})
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

// Terminal manages CLI input/output and styling.
type Terminal struct {
	mu      sync.Mutex
	spinner *spinnerRunner
}

// NewTerminal creates a new Terminal.
func NewTerminal() *Terminal {
	return &Terminal{}
}

// PrintTitle prints the application title.
func (t *Terminal) PrintTitle() {
	_, _ = fmt.Fprintln(os.Stdout, titleStyle.Render("Artoo Agent")+" - Type 'quit' to exit")
}

// ReadInput reads a line of input from the user.
func (t *Terminal) ReadInput() (string, error) {
	m := newInputModel()
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	if im, ok := finalModel.(inputModel); ok {
		return strings.TrimSpace(im.value), nil
	}

	return "", nil
}

// PrintAssistant prints assistant text with Claude styling.
func (t *Terminal) PrintAssistant(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s: %s\n", claudeStyle.Render("Claude"), text)
}

// PrintError prints an error message in error styling.
func (t *Terminal) PrintError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s\n", errorStyle.Render(fmt.Sprintf("Error: %v", err)))
}

// ShowSpinner displays a spinner with a message and returns a function to stop it.
func (t *Terminal) ShowSpinner(message string) func() {
	t.mu.Lock()
	t.spinner = newSpinner(message)
	t.mu.Unlock()
	t.spinner.start()

	return func() {
		t.mu.Lock()
		if t.spinner != nil {
			t.mu.Unlock()
			t.spinner.stop()
			t.mu.Lock()
			t.spinner = nil
		}
		t.mu.Unlock()
	}
}

// Implement agent.Callbacks interface

// OnThinking is called when the agent starts thinking.
func (t *Terminal) OnThinking() {
	t.mu.Lock()
	spinner := newSpinner("Thinking...")
	t.spinner = spinner
	t.mu.Unlock()
	spinner.start()
}

// OnThinkingDone is called when the API response is received.
func (t *Terminal) OnThinkingDone() {
	t.mu.Lock()
	spinner := t.spinner
	t.spinner = nil
	t.mu.Unlock()
	if spinner != nil {
		spinner.stop()
	}
}

// OnText is called when the assistant produces text.
func (t *Terminal) OnText(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s: %s\n", claudeStyle.Render("Claude"), text)
}

// OnToolCall is called when the assistant calls a tool.
func (t *Terminal) OnToolCall(name string, input string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s: %s\n", claudeStyle.Render("Tool"), name+": "+input)
}

// OnToolResult is called after a tool completes.
func (t *Terminal) OnToolResult(name string, _ string, isError bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	status := "OK"
	if isError {
		status = "ERROR"
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s\n", debugStyle.Render(fmt.Sprintf("[%s] %s", status, name)))
}
