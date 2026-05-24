package ui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ascarter/gh-tool/internal/tool"
)

// LiveReporter is a tool.Reporter backed by a Bubble Tea program. It owns
// stdout while the program runs and renders a multi-row spinner view: each
// in-flight tool is one row showing its current stage. When the batch
// finishes, all result lines (Done/Fail, with any buffered warnings) are
// printed to the terminal after the spinner view exits.
//
// The reporter is designed for parallel install/upgrade batches. It is
// safe to call from multiple goroutines.
type LiveReporter struct {
	prog      *tea.Program
	out       io.Writer
	startWg   sync.WaitGroup
	doneCh    chan struct{}
	stopOnce  sync.Once
	mu        sync.Mutex
	warns     map[string][]string
	completed []string
}

// NewLiveReporter constructs (but does not start) a live reporter. Call
// Launch to start the Bubble Tea program, then Stop when the batch finishes.
func NewLiveReporter() *LiveReporter {
	return &LiveReporter{
		out:    os.Stdout,
		doneCh: make(chan struct{}),
		warns:  map[string][]string{},
	}
}

var _ tool.Reporter = (*LiveReporter)(nil)

// Launch starts the Bubble Tea program in the background.
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

// Stop signals the program to quit, waits for it to flush its final frame,
// then prints all accumulated result lines to the terminal. Safe to call
// multiple times — only the first call does work.
func (r *LiveReporter) Stop() {
	if r.prog == nil {
		return
	}
	r.stopOnce.Do(func() {
		r.startWg.Wait()
		r.prog.Send(quitMsg{})
		<-r.doneCh
		// Print results after the live view has fully exited so they are never
		// overwritten by the spinner's clear-on-quit erase sequence.
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, line := range r.completed {
			fmt.Fprintln(r.out, line)
		}
	})
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

func (r *LiveReporter) Warn(name, msg string) {
	r.mu.Lock()
	r.warns[name] = append(r.warns[name], msg)
	r.mu.Unlock()
}

func (r *LiveReporter) Done(name, tag string) {
	r.mu.Lock()
	warns := r.warns[name]
	delete(r.warns, name)
	line := Success(IconSuccess) + " Installed " + name
	if tag != "" {
		line += " (" + tag + ")"
	}
	r.completed = append(r.completed, prependWarns(warns, name, line))
	r.mu.Unlock()
	r.send(doneMsg{name: name})
}

func (r *LiveReporter) Fail(name string, err error) {
	r.mu.Lock()
	warns := r.warns[name]
	delete(r.warns, name)
	line := Error(IconFailure) + " " + name + ": " + err.Error()
	r.completed = append(r.completed, prependWarns(warns, name, line))
	r.mu.Unlock()
	r.send(failMsg{name: name})
}

// prependWarns formats any buffered warnings for name and joins them with the
// terminal result line so the whole block appears as one entry.
func prependWarns(warns []string, name, terminal string) string {
	if len(warns) == 0 {
		return terminal
	}
	var b strings.Builder
	for _, w := range warns {
		b.WriteString(WarnLabel(IconWarn+" Warning:") + " " + name + ": " + w + "\n")
	}
	b.WriteString(terminal)
	return b.String()
}

// ----- model + messages ---------------------------------------------------

type startMsg struct{ name string }
type stageMsg struct{ name, stage string }
type doneMsg struct{ name string }
type failMsg struct{ name string }
type quitMsg struct{}
type tickMsg time.Time

// row tracks the state of one tool in the live view.
type row struct {
	name  string
	stage string
}

type liveModel struct {
	rows     map[string]*row
	order    []string // insertion order for stable rendering
	tick     int
	quitting bool
	width    int
}

func newLiveModel() *liveModel {
	return &liveModel{rows: map[string]*row{}}
}

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
	case doneMsg:
		m.finishRow(msg.name)
		return m, nil
	case failMsg:
		m.finishRow(msg.name)
		return m, nil
	case quitMsg:
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
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
}

func (m *liveModel) View() string {
	if len(m.order) == 0 {
		return ""
	}
	frame := spinnerFrames[m.tick%len(spinnerFrames)]
	spin := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(frame)

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
