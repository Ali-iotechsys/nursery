// Package nursery implements "structured concurrency" in Go.
//
// It's based on this blog post: https://vorpus.org/blog/notes-on-structured-concurrency-or-go-statement-considered-harmful/
package nursery

import (
	"context"
	"sync"
	"time"
)

// ConcurrentJob contains procedural code that can run concurrently to another.
// Please ensure that you're listening to `context.Done()` - at which point you're required to clean up and exit.
// Publish any errors into the error channel but note that only the first error across the jobs will be returned.
// Finally ensure that you're not unsafely modifying shared state without protection and using go's built in
// channels for communicating rather than sharing memory.
type ConcurrentJob func(context.Context, chan error)

// RunConcurrently runs jobs concurrently until all jobs have either finished or any one job encountered an error.
func RunConcurrently(jobs []ConcurrentJob) error {
	var result error

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 10)
	waitForErrCompletion := sync.WaitGroup{}
	waitForErrCompletion.Add(1)
	go func() {
		result = cancelOnFirstError(cancel, errCh)
		waitForErrCompletion.Done()
	}()

	runJobsUntilAllDone(ctx, jobs, errCh)

	close(errCh)
	waitForErrCompletion.Wait()

	return result
}

// RunUntilFirstCompletion runs jobs concurrently until atleast one job has finished or any job has encountered an error.
func RunUntilFirstCompletion(jobs []ConcurrentJob) error {
	var result error

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 10)
	waitForErrCompletion := sync.WaitGroup{}
	waitForErrCompletion.Add(1)
	go func() {
		result = cancelOnFirstError(cancel, errCh)
		waitForErrCompletion.Done()
	}()

	runJobsUntilAtleastOneDone(ctx, cancel, jobs, errCh)

	close(errCh)
	waitForErrCompletion.Wait()

	return result
}

// RunConcurrentlyWithTimeout runs jobs concurrently until all jobs have either finished or any one job encountered an error.
// or the timeout has expired
func RunConcurrentlyWithTimeout(jobs []ConcurrentJob, timeout time.Duration) error {
	var result error

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 10)
	waitForErrCompletion := sync.WaitGroup{}
	waitForErrCompletion.Add(1)
	go func() {
		result = cancelOnFirstError(cancel, errCh)
		waitForErrCompletion.Done()
	}()

	timeoutTimer := time.NewTimer(timeout)
	jobCompletionCh := make(chan struct{})

	go func() {
		runJobsUntilAllDone(ctx, jobs, errCh)
		jobCompletionCh <- struct{}{}
	}()

	select {
	case <-timeoutTimer.C:
		cancel()
		<-jobCompletionCh
	case <-jobCompletionCh:
		timeoutTimer.Stop()
	}

	close(errCh)
	waitForErrCompletion.Wait()

	return result
}

// RunUntilFirstCompletionWithTimeout runs jobs concurrently until atleast one job has finished or any job has encountered an error
// or the timeout has expired.
func RunUntilFirstCompletionWithTimeout(jobs []ConcurrentJob, timeout time.Duration) error {
	var result error

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 10)
	waitForErrCompletion := sync.WaitGroup{}
	waitForErrCompletion.Add(1)
	go func() {
		result = cancelOnFirstError(cancel, errCh)
		waitForErrCompletion.Done()
	}()

	timeoutTimer := time.NewTimer(timeout)
	jobCompletionCh := make(chan struct{})

	go func() {
		runJobsUntilAtleastOneDone(ctx, cancel, jobs, errCh)
		jobCompletionCh <- struct{}{}
	}()

	select {
	case <-timeoutTimer.C:
		cancel()
		<-jobCompletionCh
	case <-jobCompletionCh:
		timeoutTimer.Stop()
	}

	close(errCh)
	waitForErrCompletion.Wait()

	return result
}

func cancelOnFirstError(cancel context.CancelFunc, errCh chan error) error {
	err := <-errCh
	if err == nil {
		return nil
	}
	cancel()
	// drain the errCh so we don't block producers
	for range errCh {
	}
	return err
}

func runJobsUntilAllDone(ctx context.Context, jobs []ConcurrentJob, errCh chan error) {
	wg := sync.WaitGroup{}
	for _, job := range jobs {
		wg.Add(1)
		go func(job ConcurrentJob) {
			job(ctx, errCh)
			wg.Done()
		}(job)
	}
	wg.Wait()
}

func runJobsUntilAtleastOneDone(ctx context.Context, cancel context.CancelFunc, jobs []ConcurrentJob, errCh chan error) {
	wg := sync.WaitGroup{}
	for _, job := range jobs {
		wg.Add(1)
		go func(job ConcurrentJob) {
			job(ctx, errCh)
			cancel()
			wg.Done()
		}(job)
	}
	wg.Wait()
}
