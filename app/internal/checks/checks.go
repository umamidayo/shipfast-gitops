// Package checks defines status checks, runs them, and aggregates the results.
package checks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"time"
)

const (
	TypeHTTP = "http"
	TypeTCP  = "tcp"
	TypeDNS  = "dns"
)

const (
	StatusUp       = "up"
	StatusDown     = "down"
	StatusDegraded = "degraded"
	StatusPending  = "pending"
)

// Result is the outcome of one check run.
type Result struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Target    string    `json:"target"`
	Critical  bool      `json:"critical"`
	Status    string    `json:"status"`
	LatencyMS int64     `json:"latencyMs"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt,omitzero"`
}

// Redirects are reported as-is rather than followed, so a 3xx counts as "up"
// under the default status range.
var httpClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Run executes a single check, bounded by its timeout.
func Run(ctx context.Context, s Spec) Result {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout.Duration)
	defer cancel()

	start := time.Now()
	var err error
	switch s.Type {
	case TypeHTTP:
		err = checkHTTP(ctx, s)
	case TypeTCP:
		err = checkTCP(ctx, s.Target)
	case TypeDNS:
		err = checkDNS(ctx, s.Target)
	default:
		err = fmt.Errorf("unknown check type %q", s.Type)
	}

	r := pending(s)
	r.Status = StatusUp
	r.LatencyMS = time.Since(start).Milliseconds()
	r.CheckedAt = start.UTC()
	if err != nil {
		r.Status = StatusDown
		r.Error = err.Error()
	}
	return r
}

func pending(s Spec) Result {
	return Result{Name: s.Name, Type: s.Type, Target: s.Target, Critical: s.Critical, Status: StatusPending}
}

func checkHTTP(ctx context.Context, s Spec) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "status-check")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	if len(s.ExpectStatus) == 0 {
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return nil
		}
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if slices.Contains(s.ExpectStatus, resp.StatusCode) {
		return nil
	}
	return fmt.Errorf("unexpected status %d, want one of %v", resp.StatusCode, s.ExpectStatus)
}

func checkTCP(ctx context.Context, target string) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return err
	}
	return conn.Close()
}

func checkDNS(ctx context.Context, host string) error {
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return err
	}
	if len(addrs) == 0 {
		return errors.New("no addresses returned")
	}
	return nil
}
