package runstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	scenario "impact/internal/scenario_reader"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type Run struct {
	ID             string           `json:"id"`
	CreatedAt      time.Time        `json:"created_at,omitempty"`
	Target         string           `json:"target"`
	Timeout        string           `json:"timeout"`
	CleanupTimeout string           `json:"cleanup_timeout"`
	Cleanup        scenario.Command `json:"cleanup"`
	Entries        []Entry          `json:"entries"`
}

func New(s scenario.Scenario) (Run, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return Run{}, err
	}
	id := hex.EncodeToString(b[:])
	r := Run{ID: id, CreatedAt: time.Now().UTC(), Target: s.Target, Timeout: s.Timeout, CleanupTimeout: s.CleanupTimeout, Cleanup: s.Cleanup, Entries: make([]Entry, s.Prepare.Count)}
	for i := range r.Entries {
		r.Entries[i] = Entry{fmt.Sprintf("impact:%s:%d", id, i), fmt.Sprintf("value:%s:%d", id, i)}
	}
	return r, nil
}
func path(id string) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") {
		return "", fmt.Errorf("invalid run ID")
	}
	return filepath.Join("runs", id+".json"), nil
}
func Save(r Run) error {
	p, err := path(r.ID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll("runs", 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(f).Encode(r); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func Load(id string) (Run, error) {
	var r Run
	p, err := path(id)
	if err != nil {
		return r, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if r.ID != id || r.Target == "" || len(r.Entries) == 0 {
		return r, fmt.Errorf("invalid run manifest")
	}
	for _, v := range []string{r.Timeout, r.CleanupTimeout} {
		d, e := time.ParseDuration(v)
		if e != nil || d <= 0 {
			return r, fmt.Errorf("invalid manifest timeout")
		}
	}
	for _, e := range r.Entries {
		if strings.ContainsAny(e.Key+e.Value+r.ID, "\r\n") {
			return r, fmt.Errorf("invalid manifest entry")
		}
	}
	return r, r.Cleanup.Validate()
}
func Remove(id string) error {
	p, err := path(id)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// List returns saved runs in filename order, reporting unreadable manifests
// without hiding other runs. Older manifests may have no creation timestamp.
func List() ([]Run, error) {
	entries, err := os.ReadDir("runs")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var runs []Run
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		r, err := Load(id)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %s: %w", id, err))
			continue
		}
		runs = append(runs, r)
	}
	return runs, errors.Join(errs...)
}
