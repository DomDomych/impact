package runner

import (
	"bufio"
	"context"
	"fmt"
	"impact/internal/runstore"
	scenario "impact/internal/scenario_reader"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLifecycle(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	for _, mode := range []string{"normal", "prepare failure", "cancel", "cleanup retry"} {
		t.Run(mode, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			keys := map[string]bool{}
			var wg sync.WaitGroup
			done := make(chan struct{})
			rejectCleanup := mode == "cleanup retry"
			go func() {
				defer close(done)
				for {
					c, e := ln.Accept()
					if e != nil {
						return
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer c.Close()
						scan := bufio.NewScanner(c)
						for scan.Scan() {
							fields := strings.Fields(scan.Text())
							if len(fields) < 2 {
								return
							}
							mu.Lock()
							reply := "OK"
							switch fields[0] {
							case "SET":
								keys[fields[1]] = true
								if mode == "prepare failure" {
									reply = "ERR"
								}
							case "DEL":
								if rejectCleanup {
									reply = "ERR"
								} else {
									delete(keys, fields[1])
								}
							}
							mu.Unlock()
							fmt.Fprintln(c, reply)
						}
					}()
				}
			}()
			defer func() { ln.Close(); <-done; wg.Wait() }()
			s := scenario.Scenario{Target: ln.Addr().String(), Connections: 3, Duration: "60ms", Timeout: "1s", CleanupTimeout: "1s", Prepare: scenario.Command{Template: "SET {{key}} {{value}}", Count: 4, ExpectPrefix: "OK"}, Commands: []scenario.Command{{Template: "GET {{key}}", Weight: 1, ExpectPrefix: "OK"}}, Cleanup: scenario.Command{Template: "DEL {{key}}", ExpectPrefix: "OK"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				time.AfterFunc(20*time.Millisecond, cancel)
			}
			result, err := Run(ctx, s)
			if mode == "normal" {
				if err != nil || result.Stats.Completed == 0 {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			} else if err == nil {
				t.Fatal("expected failure")
			}
			if mode == "cleanup retry" {
				r, e := runstore.Load(result.RunID)
				if e != nil {
					t.Fatal(e)
				}
				mu.Lock()
				rejectCleanup = false
				mu.Unlock()
				if e = Cleanup(context.Background(), r); e != nil {
					t.Fatal(e)
				}
			}
			mu.Lock()
			remaining := len(keys)
			mu.Unlock()
			if remaining != 0 {
				t.Fatalf("%d keys remain", remaining)
			}
			files, e := filepath.Glob("runs/*.json")
			if e != nil || len(files) != 0 {
				t.Fatalf("remaining manifests %v: %v", files, e)
			}
		})
	}
}
