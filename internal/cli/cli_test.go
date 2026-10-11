package cli

import (
	"bytes"
	"context"
	"impact/internal/runstore"
	scenario "impact/internal/scenario_reader"
	"os"
	"strings"
	"testing"
	"time"
)

func TestList(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Error(err)
		}
	})
	var out, errOut bytes.Buffer
	list := func() int {
		out.Reset()
		errOut.Reset()
		return Execute(context.Background(), []string{"list"}, &out, &errOut)
	}
	if code := list(); code != 0 || !strings.Contains(out.String(), "No saved runs") {
		t.Fatalf("empty list: code=%d out=%s err=%s", code, &out, &errOut)
	}
	r := runstore.Run{ID: "older", Target: "localhost:8080", Timeout: "1s", CleanupTimeout: "1s", Cleanup: scenario.Command{Template: "DEL {{key}}"}, Entries: []runstore.Entry{{Key: "test", Value: "value"}}}
	if err := runstore.Save(r); err != nil {
		t.Fatal(err)
	}
	r.ID = "newer"
	r.CreatedAt = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	if err := runstore.Save(r); err != nil {
		t.Fatal(err)
	}
	if code := list(); code != 0 {
		t.Fatalf("list: %d %s", code, &errOut)
	}
	for _, want := range []string{"older", "newer", "unknown", "2026-10-11T12:00:00Z", "localhost:8080", "KEYS"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, &out)
		}
	}
	if err := os.WriteFile("runs/broken.json", []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := list(); code != 1 || !strings.Contains(errOut.String(), "broken") || !strings.Contains(out.String(), "newer") {
		t.Fatalf("broken manifest: code=%d out=%s err=%s", code, &out, &errOut)
	}
}
