// Package api exposes the status-check HTTP API.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/shipfast/status-check/internal/checks"
)

type server struct {
	runner  *checks.Runner
	version string
}

// New returns the HTTP handler for the API.
//
//	GET /healthz                 liveness (process is serving)
//	GET /readyz                  readiness (first round of checks has completed)
//	GET /metrics                 Prometheus metrics
//	GET /api/v1/version          build version
//	GET /api/v1/status           aggregated report; 503 when overall status is "down"
//	GET /api/v1/checks           configured checks
//	GET /api/v1/checks/{name}    latest result for one check
//
// /status and /checks/{name} accept ?refresh=true to run checks now instead of
// returning the cached result.
func New(runner *checks.Runner, version string) http.Handler {
	s := &server{runner: runner, version: version}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /api/v1/version", s.versionInfo)
	mux.HandleFunc("GET /api/v1/status", s.status)
	mux.HandleFunc("GET /api/v1/checks", s.listChecks)
	mux.HandleFunc("GET /api/v1/checks/{name}", s.getCheck)
	return logRequests(mux)
}

func (s *server) index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "status-check",
		"version": s.version,
		"endpoints": []string{
			"/healthz", "/readyz", "/metrics", "/api/v1/version",
			"/api/v1/status", "/api/v1/checks", "/api/v1/checks/{name}",
		},
	})
}

func (s *server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) readyz(w http.ResponseWriter, _ *http.Request) {
	if !s.runner.Ready() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "starting"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *server) versionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.version})
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	var rep checks.Report
	if refresh(r) {
		rep = s.runner.RunAll(r.Context())
	} else {
		rep = s.runner.Report()
	}
	code := http.StatusOK
	if rep.Status == checks.StatusDown {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, struct {
		Version string `json:"version"`
		checks.Report
	}{s.version, rep})
}

func (s *server) listChecks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"checks": s.runner.Specs()})
}

func (s *server) getCheck(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var (
		res checks.Result
		ok  bool
	)
	if refresh(r) {
		res, ok = s.runner.RunOne(r.Context(), name)
	} else {
		res, ok = s.runner.Latest(name)
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("check %q not found", name)})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) metrics(w http.ResponseWriter, _ *http.Request) {
	rep := s.runner.Report()
	var b strings.Builder
	b.WriteString("# HELP status_check_up Whether the check passed on its last run (1 up, 0 down).\n")
	b.WriteString("# TYPE status_check_up gauge\n")
	for _, c := range rep.Checks {
		if c.Status == checks.StatusPending {
			continue
		}
		up := 0
		if c.Status == checks.StatusUp {
			up = 1
		}
		fmt.Fprintf(&b, "status_check_up{name=%q,type=%q,critical=\"%t\"} %d\n", c.Name, c.Type, c.Critical, up)
	}
	b.WriteString("# HELP status_check_latency_seconds Duration of the check's last run.\n")
	b.WriteString("# TYPE status_check_latency_seconds gauge\n")
	for _, c := range rep.Checks {
		if c.Status == checks.StatusPending {
			continue
		}
		fmt.Fprintf(&b, "status_check_latency_seconds{name=%q} %g\n", c.Name, float64(c.LatencyMS)/1000)
	}
	b.WriteString("# HELP status_check_overall_up 1 unless a critical check is down.\n")
	b.WriteString("# TYPE status_check_overall_up gauge\n")
	overall := 1
	if rep.Status == checks.StatusDown {
		overall = 0
	}
	fmt.Fprintf(&b, "status_check_overall_up %d\n", overall)
	fmt.Fprintf(&b, "# TYPE status_check_build_info gauge\nstatus_check_build_info{version=%q} 1\n", s.version)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func refresh(r *http.Request) bool { return r.URL.Query().Get("refresh") == "true" }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests logs every request except probes, which would drown out the rest.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.code, "duration_ms", time.Since(start).Milliseconds())
	})
}
