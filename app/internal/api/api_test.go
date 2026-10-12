package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shipfast/status-check/internal/checks"
)

// newRunner returns a runner with one passing TCP check plus one failing check
// whose criticality is controlled by the caller.
func newRunner(t *testing.T, failingCritical bool) *checks.Runner {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := closed.Addr().String()
	closed.Close()

	timeout := checks.Duration{Duration: time.Second}
	return checks.NewRunner([]checks.Spec{
		{Name: "open", Type: checks.TypeTCP, Target: ln.Addr().String(), Timeout: timeout, Critical: true},
		{Name: "closed", Type: checks.TypeTCP, Target: closedAddr, Timeout: timeout, Critical: failingCritical},
	}, time.Hour)
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestReadyz(t *testing.T) {
	r := newRunner(t, false)
	h := New(r, "test")
	if code := get(t, h, "/readyz").Code; code != http.StatusServiceUnavailable {
		t.Errorf("before first run: %d, want 503", code)
	}
	r.RunAll(context.Background())
	if code := get(t, h, "/readyz").Code; code != http.StatusOK {
		t.Errorf("after first run: %d, want 200", code)
	}
}

func TestStatus(t *testing.T) {
	for _, tc := range []struct {
		critical   bool
		wantCode   int
		wantStatus string
	}{
		{false, http.StatusOK, checks.StatusDegraded},
		{true, http.StatusServiceUnavailable, checks.StatusDown},
	} {
		r := newRunner(t, tc.critical)
		r.RunAll(context.Background())
		rec := get(t, New(r, "test"), "/api/v1/status")
		if rec.Code != tc.wantCode {
			t.Errorf("critical=%v: code %d, want %d", tc.critical, rec.Code, tc.wantCode)
		}
		var body struct {
			Status  string `json:"status"`
			Version string `json:"version"`
			Summary checks.Summary
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Status != tc.wantStatus || body.Version != "test" || body.Summary.Down != 1 {
			t.Errorf("critical=%v: body %+v", tc.critical, body)
		}
	}
}

func TestStatusRefresh(t *testing.T) {
	h := New(newRunner(t, false), "test")
	if s := get(t, h, "/api/v1/status").Body.String(); !strings.Contains(s, `"pending"`) {
		t.Errorf("cached status before any run should be pending: %s", s)
	}
	if s := get(t, h, "/api/v1/status?refresh=true").Body.String(); !strings.Contains(s, `"degraded"`) {
		t.Errorf("refreshed status should be degraded: %s", s)
	}
}

func TestGetCheck(t *testing.T) {
	h := New(newRunner(t, false), "test")
	if code := get(t, h, "/api/v1/checks/nope").Code; code != http.StatusNotFound {
		t.Errorf("unknown check: %d, want 404", code)
	}
	rec := get(t, h, "/api/v1/checks/open?refresh=true")
	var res checks.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || res.Status != checks.StatusUp {
		t.Errorf("open check: %d %+v", rec.Code, res)
	}
}

func TestMetrics(t *testing.T) {
	r := newRunner(t, false)
	r.RunAll(context.Background())
	body := get(t, New(r, "v1"), "/metrics").Body.String()
	for _, want := range []string{
		`status_check_up{name="open",type="tcp",critical="true"} 1`,
		`status_check_up{name="closed",type="tcp",critical="false"} 0`,
		"status_check_overall_up 1",
		`status_check_build_info{version="v1"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q:\n%s", want, body)
		}
	}
}

func TestVersion(t *testing.T) {
	rec := get(t, New(newRunner(t, false), "abc1234"), "/api/v1/version")
	var body struct {
		Version       string    `json:"version"`
		StartedAt     time.Time `json:"startedAt"`
		UptimeSeconds *int64    `json:"uptimeSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || body.Version != "abc1234" || body.StartedAt.IsZero() || body.UptimeSeconds == nil {
		t.Errorf("version: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	New(newRunner(t, false), "test").ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/status", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v1/status: %d, want 405", rec.Code)
	}
}
