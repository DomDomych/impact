package cli

import (
	"context"
	"fmt"
	"impact/internal/runner"
	"impact/internal/runstore"
	scenario "impact/internal/scenario_reader"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

func Execute(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 1 && args[0] == "list" {
		runs, err := runstore.List()
		if len(runs) > 0 {
			w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "RUN ID\tCREATED (UTC)\tTARGET\tKEYS")
			for _, r := range runs {
				created := "unknown"
				if !r.CreatedAt.IsZero() {
					created = r.CreatedAt.UTC().Format(time.RFC3339)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", r.ID, created, r.Target, len(r.Entries))
			}
			w.Flush()
		} else if err == nil {
			fmt.Fprintln(out, "No saved runs.")
		}
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		return 0
	}
	if len(args) != 2 || (args[0] != "run" && args[0] != "cleanup") {
		fmt.Fprintln(errOut, "Usage: impact run <scenario-name> | impact cleanup <run-id> | impact list")
		return 2
	}
	if args[0] == "cleanup" {
		r, err := runstore.Load(args[1])
		if err == nil {
			err = runner.Cleanup(ctx, r)
		}
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		fmt.Fprintln(out, "Cleanup complete")
		return 0
	}
	name := args[1]
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		fmt.Fprintln(errOut, "invalid scenario name")
		return 2
	}
	s, err := scenario.Load(filepath.Join("scenarios", strings.TrimSuffix(name, ".json")+".json"))
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	result, err := runner.Run(ctx, s)
	fmt.Fprintf(out, "Run: %s\nElapsed: %s\nSuccessful responses: %d\nFailures: %d\n", result.RunID, result.Elapsed.Round(time.Millisecond), result.Stats.Completed, result.Stats.Failed)
	if result.Elapsed > 0 {
		fmt.Fprintf(out, "Responses/sec: %.2f\np50=%s p95=%s p99=%s\n", float64(result.Stats.Completed)/result.Elapsed.Seconds(), result.Stats.Percentile(50), result.Stats.Percentile(95), result.Stats.Percentile(99))
	}
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}
