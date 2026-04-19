package ui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ascarter/gh-tool/internal/tool"
)

// LiveReporter is a tool.Reporter backed by a Bubble Tea program. It owns
// stdout while the program runs and renders a multi-row spinner view: each
// in-flight tool is one row showing its current stage; finished tools are
// flushed above the live area as a static line so the scrollback stays
// useful after the run.
//
// The reporter is designed for parallel install/upgrade batches. It is
// safe to call from multiple goroutines: every event method dispatches a
// tea message via program.Send.
type LiveReporter struct {
	prog    *tea.Program
	out     io.Writer
	startWg sync.WaitGroup
	doneCh  chan struct{}
}

// NewLiveReporter constructs (but does not start) a live reporter. Call
// Start to launch the Bubble Tea program in the background, then Stop when
// the batch finishes.
func NewLiveReporter() *LiveReporter {
	return &LiveReporter{out: os.Stdout, doneCh: make(chan struct{})}
}

var _ tool.Reporter = (*LiveReporter)(nil)

// Launch starts the Bubble Tea program in the background. It is named
// Launch (rather than Start) to avoid colliding with the Start method on
// the tool.Reporter interface.
func (r *LiveReporter) Launch() error {
	m := newLiveModel()
	r.prog = tea.NewProgram(m, tea.WithOutput(r.out))
	r.startWg.Add(1)
	go func() {
		// Ignore tea.Run error: a TTY race during teardown should not
		// fail the install batch. The model is purely cosmetic.
		_, _ = r.prog.Run()
		close(r.doneCh)
	}()
	// Give bubbletea a beat to install its renderer before the first
	// Send arrives. Without this the first frames can be lost during the
	// race between the goroutine spinning up and the caller emitting
	// events.
	time.Sleep(20 * time.Millisecond)
	r.startWg.Done()
	return nil
}

// Stop signals the program to quit and waits for it to flush its final
// frame to the terminal.
func (r *LiveReporter) Stop() {
	if r.prog == nil {
		return
	}
	r.startWg.Wait()
	r.prog.Send(quitMsg{})
	<-r.doneCh
}

func (r *LiveReporter) send(msg tea.Msg) {
	if r.prog == nil {
		return
	}
	r.startWg.Wait()
	r.prog.Send(msg)
}

func (r *LiveReporter) Start(name string)      { r.send(startMsg{name: name}) }
func (r *LiveReporter) Stage(name, msg string) { r.send(stageMsg{name: name, stage: msg}) }
func (r *LiveReporter) Warn(name, msg string)  { r.send(warnMsg{name: name, msg: msg}) }
func (r *LiveReporter) Done(name, tag string)  { r.send(doneMsg{name: name, tag: tag}) }
func (r *LiveReporter) Fail(name string, err error) {
	r.send(failMsg{name: name, err: err})
}

// ----- model + messages ---------------------------------------------------

type startMsg struct{ name string }
type stageMsg struct{ name, stage string }
type warnMsg struct{ name, msg string }
type doneMsg struct{ name, tag string }
type failMsg struct {
	name string
	err  error
}
type quitMsg struct{}
type tickMsg time.Time

// row tracks the state of one tool in the live view.
type row struct {
	name  string
	stage string
	warns []string
}

type liveModel struct {
	rows     map[string]*row
	order    []string // insertion order for stable rendering
	finished int
	failed   int
	tick     int
	quitting bool
	width    int
}

func newLiveModel() *liveModel {
	return &liveModel{rows: map[string]*row{}}
}

// spinnerFrames is a small braille spinner. Bubble Tea's bubbles/spinner
// component would also work but pulling the full subpackage is overkill
// for a single animation we tick ourselves.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m *liveModel) Init() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *liveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tickMsg:
		m.tick++
		if m.quitting {
			return m, tea.Quit
		}
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
	case startMsg:
		if _, ok := m.rows[msg.name]; !ok {
			m.rows[msg.name] = &row{name: msg.name, stage: "starting"}
			m.order = append(m.order, msg.name)
		}
		return m, nil
	case stageMsg:
		if r, ok := m.rows[msg.name]; ok {
			r.stage = msg.stage
		}
		return m, nil
	case warnMsg:
		// Buffer warns so they print together with the tool's terminal
		// line (Done/Fail) instead of interleaving with other tools'
		// output. The row is created lazily here in case Warn arrives
		// before Start (rare but harmless).
		r, ok := m.rows[msg.name]
		if !ok {
			r = &row{name: msg.name}
			m.rows[msg.name] = r
			m.order = append(m.order, msg.name)
		}
		r.warns = append(r.warns, msg.msg)
		return m, nil
	case doneMsg:
		warns := m.flushWarns(msg.name)
		m.finishRow(msg.name)
		line := Success(IconSuccess) + " Installed " + msg.name
		if msg.tag != "" {
			line += " (" + msg.tag + ")"
		}
		return m, printAbove(joinTool(warns, msg.name, line))
	case failMsg:
		warns := m.flushWarns(msg.name)
		m.finishRow(msg.name)
		m.failed++
		line := Error(IconFailure) + " " + msg.name + ": " + msg.err.Error()
		return m, printAbove(joinTool(warns, msg.name, line))
	case quitMsg:
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

// flushWarns returns the accumulated warnings for a tool and clears them.
func (m *liveModel) flushWarns(name string) []string {
	r, ok := m.rows[name]
	if !ok {
		return nil
	}
	w := r.warns
	r.warns = nil
	return w
}

// joinTool prefixes a slice of warnings with the styled "⚠ Warning:" label
// and the tool name, then appends the terminal line so the whole block
// appears as one chunk in the scrollback.
func joinTool(warns []string, name, terminal string) string {
	if len(warns) == 0 {
		return terminal
	}
	out := ""
	for _, w := range warns {
		out += WarnLabel(IconWarn+" Warning:") + " " + name + ": " + w + "\n"
	}
	return out + terminal
}

func (m *liveModel) finishRow(name string) {
	if _, ok := m.rows[name]; !ok {
		return
	}
	delete(m.rows, name)
	for i, n := range m.order {
		if n == name {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.finished++
}

// printAbove returns a tea.Cmd that prints a line in the scrollback above
// the live view, leaving the terminal scrollable and copyable after the
// run completes.
func printAbove(line string) tea.Cmd {
	return tea.Printf("%s", line)
}

func (m *liveModel) View() string {
	if len(m.order) == 0 {
		return ""
	}
	frame := spinnerFrames[m.tick%len(spinnerFrames)]
	spin := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(frame)

	// Stable order for rendering — insertion order keeps recently-started
	// rows where the user expects them.
	names := make([]string, 0, len(m.order))
	names = append(names, m.order...)
	sort.SliceStable(names, func(i, j int) bool { return false })

	var lines []string
	for _, n := range names {
		r := m.rows[n]
		stage := r.stage
		if stage == "" {
			stage = "working"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %s", spin, Bold(n), Muted(IconArrow), stage))
	}
	return joinLines(lines)
}

func joinLines(ls []string) string {
	if len(ls) == 0 {
		return ""
	}
	out := ls[0]
	for _, l := range ls[1:] {
		out += "\n" + l
	}
	return out
}
