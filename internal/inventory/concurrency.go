package inventory

import (
	"context"
	"sync"
)

// runFailFast runs fn(ctx, i) for i in [0, n) with at most concurrency
// goroutines in flight (spec section 23: bounded worker pool, never one
// goroutine per resource). The first error cancels the derived context so
// in-flight and not-yet-started work stops promptly, and is returned once
// all launched goroutines have finished.
func runFailFast(ctx context.Context, concurrency, n int, fn func(context.Context, int) error) error {
	if concurrency < 1 {
		concurrency = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error

loop:
	for i := 0; i < n; i++ {
		select {
		case <-ctx.Done():
			break loop
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(ctx, i); err != nil {
				once.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}(i)
	}
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

// runBestEffort runs fn(ctx, i) for i in [0, n) with at most concurrency
// goroutines in flight, and does not abort on error: fn is responsible for
// recording its own per-item failure (see Resource.CountError). Launching
// new work stops once ctx is cancelled, but already-started work is always
// waited on before returning.
func runBestEffort(ctx context.Context, concurrency, n int, fn func(context.Context, int)) {
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

loop:
	for i := 0; i < n; i++ {
		select {
		case <-ctx.Done():
			break loop
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(ctx, i)
		}(i)
	}
	wg.Wait()
}
