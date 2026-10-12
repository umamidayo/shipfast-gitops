package checks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"time"
)

const defaultTimeout = 3 * time.Second

// Names are used in URLs and Prometheus labels, so keep them simple.
var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

// Duration is a time.Duration that (un)marshals as a Go duration string, e.g. "2s".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf(`duration must be a string such as "2s"`)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// Spec describes a single check.
type Spec struct {
	Name   string `json:"name"`
	Type   string `json:"type"`   // http | tcp | dns
	Target string `json:"target"` // URL, host:port, or hostname
	// Timeout bounds a single run of the check. Defaults to 3s.
	Timeout Duration `json:"timeout"`
	// Critical checks turn the overall status "down" when they fail;
	// non-critical failures only make it "degraded".
	Critical bool `json:"critical"`
	// ExpectStatus lists acceptable HTTP status codes. Defaults to 200-399.
	ExpectStatus []int `json:"expectStatus,omitempty"`
}

type Config struct {
	Checks []Spec `json:"checks"`
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checks config: %w", err)
	}
	return ParseConfig(b)
}

// ParseConfig decodes and validates a checks config, filling in defaults.
func ParseConfig(b []byte) (*Config, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse checks config: %w", err)
	}

	seen := make(map[string]bool, len(cfg.Checks))
	for i := range cfg.Checks {
		s := &cfg.Checks[i]
		if !validName.MatchString(s.Name) {
			return nil, fmt.Errorf("checks[%d]: invalid name %q (use letters, digits, '.', '_' or '-')", i, s.Name)
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("checks[%d]: duplicate name %q", i, s.Name)
		}
		seen[s.Name] = true
		if err := validateTarget(s.Type, s.Target); err != nil {
			return nil, fmt.Errorf("check %q: %w", s.Name, err)
		}
		if s.Timeout.Duration <= 0 {
			s.Timeout.Duration = defaultTimeout
		}
	}
	return &cfg, nil
}

func validateTarget(typ, target string) error {
	if target == "" {
		return fmt.Errorf("target is required")
	}
	switch typ {
	case TypeHTTP:
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("http target must be an absolute http(s) URL, got %q", target)
		}
	case TypeTCP:
		if _, _, err := net.SplitHostPort(target); err != nil {
			return fmt.Errorf("tcp target must be host:port, got %q", target)
		}
	case TypeDNS:
	default:
		return fmt.Errorf("unknown type %q (want http, tcp or dns)", typ)
	}
	return nil
}
