package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"time"
)

type result struct {
	completed, failed int64
	latencies []time.Duration
	err error
}

func main() {
	target := flag.String("target", "127.0.0.1:8080", "TCP server address")
	clients := flag.Int("clients", 10, "Number of concurrent connections")
	duration := flag.Duration("duration", 10*time.Second, "Run duration")
	timeout := flag.Duration("timeout", 2*time.Second, "Connection and request timeout")
	command := flag.String("command", "GET impact:test", "Command sent by each client")
	flag.Parse()
	if *clients < 1 || *duration <= 0 || *timeout <= 0 || *command == "" || strings.ContainsAny(*command, "\r\n") {
		fmt.Fprintln(os.Stderr, "clients, duration and timeout must be positive; command must be one nonempty line")
		os.Exit(2)
	}
	parent, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(parent, *duration)
	defer cancel()
	results := make(chan result, *clients)
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < *clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- worker(ctx, *target, *command, *timeout)
		}()
	}
	wg.Wait()
	close(results)
	elapsed := time.Since(start)
	var total result
	for r := range results {
		total.completed += r.completed
		total.failed += r.failed
		total.latencies = append(total.latencies, r.latencies...)
		if r.err != nil && total.err == nil { total.err = r.err }
	}
	sort.Slice(total.latencies, func(i, j int) bool { return total.latencies[i] < total.latencies[j] })
	fmt.Printf("Elapsed: %s\nCompleted responses: %d\nTransport failures: %d\nResponses/sec: %.2f\n", elapsed.Round(time.Millisecond), total.completed, total.failed, float64(total.completed)/elapsed.Seconds())
	if len(total.latencies) > 0 {
		fmt.Printf("Sampled latency (%d samples): p50=%s p95=%s p99=%s\n", len(total.latencies), percentile(total.latencies, 50), percentile(total.latencies, 95), percentile(total.latencies, 99))
	}
	if total.err != nil { fmt.Fprintln(os.Stderr, "First transport error:", total.err) }
	if total.failed > 0 { os.Exit(1) }
}

func worker(ctx context.Context, target, command string, timeout time.Duration) result {
	var r result
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		if ctx.Err() == nil { r.failed++; r.err = err }
		return r
	}
	defer conn.Close()
	// Closing the socket interrupts a blocked request when the run ends.
	stopClose := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClose()
	reader := bufio.NewReader(conn)
	const sampleLimit = 10000
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for ctx.Err() == nil {
		started := time.Now()
		if err = conn.SetDeadline(started.Add(timeout)); err == nil {
			_, err = fmt.Fprint(conn, command, "\n")
		}
		if err == nil { err = readResponse(reader) }
		if err != nil {
			if ctx.Err() == nil { r.failed++; r.err = err }
			return r // A failed stream cannot safely be reused.
		}
		latency := time.Since(started)
		r.completed++
		// Reservoir sampling bounds memory independently of run duration.
		if len(r.latencies) < sampleLimit {
			r.latencies = append(r.latencies, latency)
		} else if index := rng.Int63n(r.completed); index < sampleLimit {
			r.latencies[index] = latency
		}
	}
	return r
}

func readResponse(reader *bufio.Reader) error {
	const maxResponse = 1024 * 1024
	size := 0
	for {
		chunk, err := reader.ReadSlice('\n')
		size += len(chunk)
		if size > maxResponse { return fmt.Errorf("response exceeds %d bytes", maxResponse) }
		if err == bufio.ErrBufferFull { continue }
		return err
	}
}

func percentile(values []time.Duration, p int) time.Duration {
	index := (len(values)*p + 99)/100 - 1
	return values[index].Round(time.Microsecond)
}