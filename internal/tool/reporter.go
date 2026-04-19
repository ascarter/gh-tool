package tool

// Reporter receives progress events from Manager operations. Implementations
// decide how to render them: a line reporter prints one event per line; a
// live TUI reporter aggregates events into a multi-row spinner view. All
// methods may be called concurrently from different goroutines (one per
// tool) — implementations must be goroutine-safe.
type Reporter interface {
	// Start signals that work has begun for a tool.
	Start(name string)
	// Stage reports a sub-step (e.g. "Downloading foo/bar v1.2.3",
	// "Verifying attestation", "Extracting archive.tar.gz").
	Stage(name, msg string)
	// Warn surfaces a non-fatal warning attached to a tool (e.g. an
	// attestation that could not be verified, or a missing man page).
	Warn(name, msg string)
	// Done signals successful completion for a tool. tag may be empty.
	Done(name, tag string)
	// Fail signals terminal failure for a tool.
	Fail(name string, err error)
}

// nopReporter discards every event. Used as the zero value so existing call
// sites that don't pass a reporter behave exactly as before (silently),
// though in practice every command supplies one.
type nopReporter struct{}

func (nopReporter) Start(string)         {}
func (nopReporter) Stage(string, string) {}
func (nopReporter) Warn(string, string)  {}
func (nopReporter) Done(string, string)  {}
func (nopReporter) Fail(string, error)   {}
