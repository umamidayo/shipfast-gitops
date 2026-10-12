package checks

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"checks":[
		{"name":"web","type":"http","target":"http://example.com","critical":true},
		{"name":"db","type":"tcp","target":"db:5432","timeout":"500ms"},
		{"name":"dns","type":"dns","target":"example.com"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Checks[0].Timeout.Duration; got != defaultTimeout {
		t.Errorf("default timeout = %v, want %v", got, defaultTimeout)
	}
	if got := cfg.Checks[1].Timeout.Duration; got != 500*time.Millisecond {
		t.Errorf("timeout = %v, want 500ms", got)
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := map[string]string{
		"bad type":        `{"checks":[{"name":"a","type":"ftp","target":"x"}]}`,
		"missing target":  `{"checks":[{"name":"a","type":"dns"}]}`,
		"duplicate":       `{"checks":[{"name":"a","type":"dns","target":"x"},{"name":"a","type":"dns","target":"y"}]}`,
		"bad name":        `{"checks":[{"name":"a b","type":"dns","target":"x"}]}`,
		"relative url":    `{"checks":[{"name":"a","type":"http","target":"/healthz"}]}`,
		"tcp no port":     `{"checks":[{"name":"a","type":"tcp","target":"db"}]}`,
		"unknown field":   `{"checks":[{"name":"a","type":"dns","target":"x","retries":3}]}`,
		"numeric timeout": `{"checks":[{"name":"a","type":"dns","target":"x","timeout":5}]}`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig([]byte(in)); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func spec(name, typ, target string) Spec {
	return Spec{Name: name, Type: typ, Target: target, Timeout: Duration{time.Second}}
}

func TestRunHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/teapot":
			w.WriteHeader(http.StatusTeapot)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	if r := Run(context.Background(), spec("ok", TypeHTTP, srv.URL+"/ok")); r.Status != StatusUp {
		t.Errorf("/ok: status %s, error %q", r.Status, r.Error)
	}
	r := Run(context.Background(), spec("err", TypeHTTP, srv.URL+"/err"))
	if r.Status != StatusDown || !strings.Contains(r.Error, "500") {
		t.Errorf("/err: status %s, error %q", r.Status, r.Error)
	}
	s := spec("teapot", TypeHTTP, srv.URL+"/teapot")
	s.ExpectStatus = []int{418}
	if r := Run(context.Background(), s); r.Status != StatusUp {
		t.Errorf("/teapot with expectStatus: status %s, error %q", r.Status, r.Error)
	}
}

func TestRunTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if r := Run(context.Background(), spec("open", TypeTCP, addr)); r.Status != StatusUp {
		t.Errorf("open port: status %s, error %q", r.Status, r.Error)
	}
	ln.Close()
	if r := Run(context.Background(), spec("closed", TypeTCP, addr)); r.Status != StatusDown {
		t.Errorf("closed port: status %s", r.Status)
	}
}

func TestRunDNS(t *testing.T) {
	if r := Run(context.Background(), spec("localhost", TypeDNS, "localhost")); r.Status != StatusUp {
		t.Errorf("localhost: status %s, error %q", r.Status, r.Error)
	}
	if r := Run(context.Background(), spec("bogus", TypeDNS, "does-not-exist.invalid")); r.Status != StatusDown {
		t.Errorf(".invalid: status %s", r.Status)
	}
}

func TestAggregate(t *testing.T) {
	up := Result{Status: StatusUp}
	down := Result{Status: StatusDown}
	critDown := Result{Status: StatusDown, Critical: true}
	pend := Result{Status: StatusPending}

	cases := []struct {
		in   []Result
		want string
	}{
		{nil, StatusUp},
		{[]Result{up, up}, StatusUp},
		{[]Result{up, pend}, StatusPending},
		{[]Result{up, down, pend}, StatusDegraded},
		{[]Result{critDown, down, up}, StatusDown},
	}
	for _, c := range cases {
		if got, _ := Aggregate(c.in); got != c.want {
			t.Errorf("Aggregate(%v) = %s, want %s", c.in, got, c.want)
		}
	}
	_, sum := Aggregate([]Result{up, down, critDown, pend})
	if sum != (Summary{Total: 4, Up: 1, Down: 2, Pending: 1}) {
		t.Errorf("summary = %+v", sum)
	}
}

func TestRunner(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	r := NewRunner([]Spec{spec("a", TypeTCP, ln.Addr().String())}, time.Hour)
	if r.Ready() {
		t.Fatal("ready before first run")
	}
	if got := r.Report().Status; got != StatusPending {
		t.Errorf("status before run = %s, want pending", got)
	}
	if got := r.RunAll(context.Background()).Status; got != StatusUp {
		t.Errorf("status after run = %s, want up", got)
	}
	if !r.Ready() {
		t.Error("not ready after first run")
	}
	if _, ok := r.Latest("missing"); ok {
		t.Error("Latest found a check that doesn't exist")
	}
}
