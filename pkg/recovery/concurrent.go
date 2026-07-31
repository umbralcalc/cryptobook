package recovery

import (
	"errors"
	"runtime"
	"sync"
)

// inParallel applies measure to every input concurrently and returns the results in
// input order, joining every error rather than reporting only the first.
//
// # Why this is safe for the numbers
//
// Each measurement here is one engine run over its own substituted config. Runs
// share no state, and each carries its own seed from the config it was rendered
// from, so a run's output depends on nothing but its own input. Order of execution
// is therefore not observable in any recorded value — the results slice is filled by
// index, never appended to, so even the ORDER of the observations a claim reports is
// unchanged. Anything drawn from a shared random stream must still be drawn before
// this is called, not inside measure; see effectiveSampleSize.
//
// # Why it exists
//
// The loops this replaced were serial only because they were written as loops. This
// package makes ~27 engine runs, and on a two-core CI runner under -race that
// serialism was the whole difference between a package that fits the test timeout
// and one that panics at it.
//
// Concurrency is capped at GOMAXPROCS because each engine run is itself
// multi-goroutine — the coordinator runs a goroutine per partition — so letting all
// of them start at once would oversubscribe the runner and hold every run's storage
// live simultaneously.
func inParallel[In, Out any](
	inputs []In,
	measure func(In) (Out, error),
) ([]Out, error) {
	results := make([]Out, len(inputs))
	failures := make([]error, len(inputs))
	limit := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wait sync.WaitGroup
	for i, input := range inputs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			results[i], failures[i] = measure(input)
		}()
	}
	wait.Wait()
	if err := errors.Join(failures...); err != nil {
		return nil, err
	}
	return results, nil
}
