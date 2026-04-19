package ui

import (
	"bytes"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	var counter atomic.Int32
	jobs := []Job{
		{Name: "a", Run: func() error { counter.Add(1); return nil }},
		{Name: "b", Run: func() error { counter.Add(1); return nil }},
		{Name: "c", Run: func() error { counter.Add(1); return nil }},
	}
	results, err := Run(jobs, 2)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if counter.Load() != 3 {
		t.Errorf("ran %d jobs, want 3", counter.Load())
	}
	if len(results) != 3 {
		t.Errorf("got %d results, want 3", len(results))
	}
	names := []string{}
	for _, r := range results {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "a,b,c" {
		t.Errorf("results=%v", names)
	}
}

func TestRunPartialFailure(t *testing.T) {
	jobs := []Job{
		{Name: "ok1", Run: func() error { return nil }},
		{Name: "fail", Run: func() error { return errors.New("boom") }},
		{Name: "ok2", Run: func() error { return nil }},
	}
	results, err := Run(jobs, 4)
	if err == nil {
		t.Fatalf("expected aggregate error, got nil")
	}
	be, ok := err.(*BatchError)
	if !ok {
		t.Fatalf("expected *BatchError, got %T", err)
	}
	if be.Failed != 1 || be.Total != 3 {
		t.Errorf("BatchError = %+v, want Failed=1 Total=3", be)
	}
	var failedNames []string
	for _, r := range results {
		if r.Err != nil {
			failedNames = append(failedNames, r.Name)
		}
	}
	if len(failedNames) != 1 || failedNames[0] != "fail" {
		t.Errorf("failed names=%v, want [fail]", failedNames)
	}
}

// TestRunConcurrency confirms the worker cap is honoured: with workers=2
// and 4 jobs that block on a channel, only 2 should be in-flight at once.
func TestRunConcurrency(t *testing.T) {
	var inflight atomic.Int32
	var maxInflight atomic.Int32
	release := make(chan struct{})

	jobs := []Job{}
	for i := 0; i < 4; i++ {
		jobs = append(jobs, Job{
			Name: "j",
			Run: func() error {
				cur := inflight.Add(1)
				for {
					m := maxInflight.Load()
					if cur <= m || maxInflight.CompareAndSwap(m, cur) {
						break
					}
				}
				<-release
				inflight.Add(-1)
				return nil
			},
		})
	}

	done := make(chan struct{})
	go func() {
		_, _ = Run(jobs, 2)
		close(done)
	}()

	// Give workers time to ramp up. 50ms is enough on any reasonable box.
	time.Sleep(50 * time.Millisecond)
	close(release)
	<-done

	if maxInflight.Load() > 2 {
		t.Errorf("max in-flight = %d, want <= 2", maxInflight.Load())
	}
}

func TestRunEmpty(t *testing.T) {
	results, err := Run(nil, 4)
	if err != nil {
		t.Errorf("err=%v", err)
	}
	if len(results) != 0 {
		t.Errorf("results=%v", results)
	}
}

func TestResolveJobs(t *testing.T) {
	if got := ResolveJobs(0); got != DefaultJobs() {
		t.Errorf("ResolveJobs(0)=%d, want %d", got, DefaultJobs())
	}
	if got := ResolveJobs(-3); got != DefaultJobs() {
		t.Errorf("ResolveJobs(-3)=%d, want %d", got, DefaultJobs())
	}
	if got := ResolveJobs(1); got != 1 {
		t.Errorf("ResolveJobs(1)=%d, want 1", got)
	}
	if got := ResolveJobs(99); got != 99 {
		t.Errorf("ResolveJobs(99)=%d, want 99", got)
	}
}

// TestLineReporterPrefixed checks that parallel mode tags every line with
// its tool name so concurrent output stays readable when piped to a file.
func TestLineReporterPrefixed(t *testing.T) {
	var buf bytes.Buffer
	r := &LineReporter{W: &buf, Prefixed: true, Verbose: true}

	// Hammer the reporter from multiple goroutines to exercise the mutex.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.Stage("tool"+itoa(n), "Downloading something")
		}(i)
	}
	wg.Wait()

	r.Warn("tool2", "missing thing")
	r.Done("tool0", "v1")
	r.Fail("tool1", errors.New("boom"))
	r.Done("tool2", "v2")

	out := buf.String()
	for _, want := range []string{"[tool0]", "[tool1]", "[tool2]", "Downloading something", "Installed tool0 (v1)", "tool1: boom", "missing thing"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestLineReporterUnprefixed (single-tool path) keeps the historical
// format: no "[name]" tag, no extra Start line.
func TestLineReporterUnprefixed(t *testing.T) {
	var buf bytes.Buffer
	r := &LineReporter{W: &buf, Prefixed: false, Verbose: true}

	r.Start("widget")
	r.Stage("widget", "Downloading widget v1")
	r.Done("widget", "v1")

	out := buf.String()
	if strings.Contains(out, "[widget]") {
		t.Errorf("unprefixed output should not contain [widget]:\n%s", out)
	}
	for _, want := range []string{"Downloading widget v1", "Installed widget (v1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Start should be silent in unprefixed mode (preserving prior UX).
	if strings.Count(out, "starting") > 0 {
		t.Errorf("Start should not print in unprefixed mode:\n%s", out)
	}
}

// TestLineReporterNonVerbose confirms Stage messages are suppressed by
// default; Warn/Done/Fail still surface.
func TestLineReporterNonVerbose(t *testing.T) {
	var buf bytes.Buffer
	r := &LineReporter{W: &buf, Prefixed: true, Verbose: false}
	r.Start("widget")
	r.Stage("widget", "Downloading widget v1")
	r.Stage("widget", "Verifying attestation for widget.tar.gz")
	r.Warn("widget", "attestation not verified")
	r.Done("widget", "v1")

	out := buf.String()
	if strings.Contains(out, "Downloading") || strings.Contains(out, "Verifying") {
		t.Errorf("non-verbose should suppress Stage:\n%s", out)
	}
	for _, want := range []string{"attestation not verified", "Installed widget (v1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
