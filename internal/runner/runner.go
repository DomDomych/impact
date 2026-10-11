package runner

import (
	"context"
	"errors"
	"fmt"
	"impact/internal/metrics"
	"impact/internal/runstore"
	scenario "impact/internal/scenario_reader"
	"impact/internal/transport"
	"math/rand"
	"sync"
	"time"
)

type Result struct {
	RunID   string
	Stats   metrics.Stats
	Elapsed time.Duration
}

func sequence(ctx context.Context, r runstore.Run, command scenario.Command) error {
	timeout, _ := time.ParseDuration(r.Timeout)
	c, err := transport.Dial(ctx, r.Target, timeout)
	if err != nil {
		return err
	}
	defer c.Close()
	for _, e := range r.Entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		if _, err = c.Request(command.Render(e.Key, e.Value, r.ID), command.ExpectPrefix); err != nil {
			return fmt.Errorf("key %s: %w", e.Key, err)
		}
	}
	return nil
}
func Cleanup(parent context.Context, r runstore.Run) error {
	d, _ := time.ParseDuration(r.CleanupTimeout)
	ctx, cancel := context.WithTimeout(parent, d)
	defer cancel()
	if err := sequence(ctx, r, r.Cleanup); err != nil {
		return fmt.Errorf("cleanup incomplete; retry with impact cleanup %s: %w", r.ID, err)
	}
	return runstore.Remove(r.ID)
}
func Run(ctx context.Context, s scenario.Scenario) (result Result, err error) {
	if err = s.Validate(); err != nil {
		return
	}
	r, err := runstore.New(s)
	if err != nil {
		return result, err
	}
	result.RunID = r.ID
	if err = runstore.Save(r); err != nil {
		return result, err
	}
	// Cleanup has its own deadline and survives cancellation of preparation or load.
	defer func() { err = errors.Join(err, Cleanup(context.Background(), r)) }()
	if err = sequence(ctx, r, s.Prepare); err != nil {
		return result, fmt.Errorf("prepare: %w", err)
	}
	d, _ := time.ParseDuration(s.Duration)
	load, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	start := time.Now()
	results := make(chan *metrics.Stats, s.Connections)
	var wg sync.WaitGroup
	for i := 0; i < s.Connections; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); stats := worker(load, s, r); results <- &stats }()
	}
	wg.Wait()
	close(results)
	result.Elapsed = time.Since(start)
	for stats := range results {
		result.Stats.Merge(*stats)
	}
	if result.Stats.Failed > 0 {
		err = fmt.Errorf("%d request failures: %w", result.Stats.Failed, result.Stats.FirstError)
	}
	if ctx.Err() != nil {
		err = errors.Join(err, ctx.Err())
	}
	return
}
func worker(ctx context.Context, s scenario.Scenario, r runstore.Run) (stats metrics.Stats) {
	timeout, _ := time.ParseDuration(s.Timeout)
	c, err := transport.Dial(ctx, s.Target, timeout)
	if err != nil {
		if ctx.Err() == nil {
			stats.Fail(err)
		}
		return
	}
	defer c.Close()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	total := 0
	for _, cmd := range s.Commands {
		total += cmd.Weight
	}
	for ctx.Err() == nil {
		n := rng.Intn(total)
		cmd := s.Commands[0]
		for _, candidate := range s.Commands {
			n -= candidate.Weight
			if n < 0 {
				cmd = candidate
				break
			}
		}
		e := r.Entries[rng.Intn(len(r.Entries))]
		start := time.Now()
		complete, err := c.Request(cmd.Render(e.Key, e.Value, r.ID), cmd.ExpectPrefix)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			stats.Fail(err)
			if !complete {
				return
			}
			continue
		}
		stats.Record(time.Since(start))
	}
	return
}
