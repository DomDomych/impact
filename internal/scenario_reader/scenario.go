package scenario_reader

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Command struct {
	Template     string `json:"template"`
	ExpectPrefix string `json:"expect_prefix,omitempty"`
	Weight       int    `json:"weight,omitempty"`
	Count        int    `json:"count,omitempty"`
}
type Scenario struct {
	Target         string    `json:"target"`
	Connections    int       `json:"connections"`
	Duration       string    `json:"duration"`
	Timeout        string    `json:"timeout"`
	CleanupTimeout string    `json:"cleanup_timeout"`
	Prepare        Command   `json:"prepare"`
	Commands       []Command `json:"commands"`
	Cleanup        Command   `json:"cleanup"`
}

func Load(path string) (Scenario, error) {
	var s Scenario
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(&s); err != nil {
		return s, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return s, fmt.Errorf("scenario must contain one JSON object")
	}
	return s, s.Validate()
}
func (s Scenario) Validate() error {
	if s.Target == "" || s.Connections < 1 || s.Prepare.Count < 1 || len(s.Commands) == 0 {
		return fmt.Errorf("target, positive connections, prepare.count and commands are required")
	}
	for _, v := range []string{s.Duration, s.Timeout, s.CleanupTimeout} {
		d, e := time.ParseDuration(v)
		if e != nil || d <= 0 {
			return fmt.Errorf("invalid positive duration: %q", v)
		}
	}
	commands := append([]Command{s.Prepare, s.Cleanup}, s.Commands...)
	for _, c := range commands {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	total := 0
	for _, c := range s.Commands {
		if c.Weight <= 0 || c.Weight > int(^uint(0)>>1)-total {
			return fmt.Errorf("command weights must be positive without overflow")
		}
		total += c.Weight
	}
	return nil
}
func (c Command) Validate() error {
	if c.Template == "" || strings.ContainsAny(c.Template, "\r\n") {
		return fmt.Errorf("command must be a nonempty single line")
	}
	rest := strings.NewReplacer("{{key}}", "", "{{value}}", "", "{{run_id}}", "").Replace(c.Template)
	if strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
		return fmt.Errorf("unknown template placeholder in %q", c.Template)
	}
	return nil
}
func (c Command) Render(key, value, id string) string {
	return strings.NewReplacer("{{key}}", key, "{{value}}", value, "{{run_id}}", id).Replace(c.Template)
}
