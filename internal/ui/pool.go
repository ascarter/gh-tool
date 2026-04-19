package ui

import (
	"runtime"
	"sync"
)

// DefaultJobs returns the default concurrency limit for parallel network
// operations: min(8, NumCPU). Capped at 8 to keep concurrent gh CLI calls
// from blowing through API rate limits on multi-tool installs.
func DefaultJobs() int {
	n := runtime.NumCPU()
	if n > 8 {
		n = 8
	}
	if n < 1 {
		n = 1
	}
	return n
}

// ResolveJobs picks a usable worker count from a user-supplied value. Values
// <= 0 fall back to DefaultJobs(). A larger requested value than the number
// of items in the batch is fine — extra workers just exit immediately.
func ResolveJobs(requested int) int {
	if requested <= 0 {
		return DefaultJobs()
	}
	return requested
}

// Job is a single unit of work in a parallel batch. Name is used in the UI
// (and to associate the result with the originating tool); Run does the
// actual work and returns a terminal error if any.
type Job struct {
	Name string
	Run  func() error
}

// Result is a Job's outcome, carried back through Run for batch summaries.
type Result struct {
	Name string
	Err  error
}

// Run executes jobs concurrently with at most workers in flight at once.
// Results are returned in completion order — callers that need stable
// ordering should sort on Name. Workers fan out goroutines but always
// return after every job has finished or panicked.
//
// Run returns nil if every job succeeded, or a non-nil aggregate error
// summarising the failures. The full per-job results slice is always
// returned so callers can render their own report.
func Run(jobs []Job, workers int) ([]Result, error) {
	if workers < 1 {
		workers = 1
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}

	results := make([]Result, 0, len(jobs))
	if len(jobs) == 0 {
		return results, nil
	}

	in := make(chan Job)
	out := make(chan Result, len(jobs))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				err := j.Run()
				out <- Result{Name: j.Name, Err: err}
			}
		}()
	}

	go func() {
		for _, j := range jobs {
			in <- j
		}
		close(in)
		wg.Wait()
		close(out)
	}()

	var failed int
	for r := range out {
		if r.Err != nil {
			failed++
		}
		results = append(results, r)
	}

	if failed == 0 {
		return results, nil
	}
	return results, &BatchError{Failed: failed, Total: len(jobs)}
}

// BatchError is returned by Run when one or more jobs fail. Callers can
// type-assert to extract counts; the message is suitable for direct display.
type BatchError struct {
	Failed int
	Total  int
}

func (e *BatchError) Error() string {
	if e.Failed == 1 {
		return "1 job failed"
	}
	return pluralCount(e.Failed) + " of " + pluralCount(e.Total) + " jobs failed"
}

// pluralCount renders an integer with a trailing word. We keep it tiny;
// fmt would pull no extra dependencies but a hand-rolled itoa stays
// allocation-light and explicit.
func pluralCount(n int) string {
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
