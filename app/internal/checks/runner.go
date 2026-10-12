package checks

import (
	"context"
	"sync"
	"time"
)

// Report is the aggregated view of all checks.
type Report struct {
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checkedAt,omitzero"`
	Summary   Summary   `json:"summary"`
	Checks    []Result  `json:"checks"`
}

type Summary struct {
	Total   int `json:"total"`
	Up      int `json:"up"`
	Down    int `json:"down"`
	Pending int `json:"pending"`
}

// Runner runs all checks on an interval and caches the latest results, so API
// requests are cheap and don't hammer the targets.
type Runner struct {
	specs    []Spec
	interval time.Duration

	mu      sync.RWMutex
	results map[string]Result
	lastRun time.Time
}

func NewRunner(specs []Spec, interval time.Duration) *Runner {
	return &Runner{specs: specs, interval: interval, results: make(map[string]Result, len(specs))}
}

// Start runs all checks immediately and then on every interval until ctx is done.
func (r *Runner) Start(ctx context.Context) {
	r.RunAll(ctx)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.RunAll(ctx)
		}
	}
}

// RunAll runs every check concurrently, stores the results and returns the new report.
func (r *Runner) RunAll(ctx context.Context) Report {
	results := make([]Result, len(r.specs))
	var wg sync.WaitGroup
	for i, s := range r.specs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = Run(ctx, s)
		}()
	}
	wg.Wait()

	r.mu.Lock()
	for _, res := range results {
		r.results[res.Name] = res
	}
	r.lastRun = time.Now().UTC()
	r.mu.Unlock()
	return r.Report()
}

// RunOne runs a single check by name and stores its result.
func (r *Runner) RunOne(ctx context.Context, name string) (Result, bool) {
	s, ok := r.Spec(name)
	if !ok {
		return Result{}, false
	}
	res := Run(ctx, s)
	r.mu.Lock()
	r.results[name] = res
	r.mu.Unlock()
	return res, true
}

// Latest returns the cached result for a check (status "pending" if it hasn't run yet).
func (r *Runner) Latest(name string) (Result, bool) {
	s, ok := r.Spec(name)
	if !ok {
		return Result{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if res, ok := r.results[name]; ok {
		return res, true
	}
	return pending(s), true
}

func (r *Runner) Spec(name string) (Spec, bool) {
	for _, s := range r.specs {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

func (r *Runner) Specs() []Spec { return r.specs }

// Ready reports whether the first full round of checks has completed.
func (r *Runner) Ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.lastRun.IsZero()
}

// Report aggregates the cached results, in config order.
func (r *Runner) Report() Report {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rep := Report{CheckedAt: r.lastRun, Checks: make([]Result, 0, len(r.specs))}
	for _, s := range r.specs {
		res, ok := r.results[s.Name]
		if !ok {
			res = pending(s)
		}
		rep.Checks = append(rep.Checks, res)
	}
	rep.Status, rep.Summary = Aggregate(rep.Checks)
	return rep
}

// Aggregate derives the overall status: down > degraded > pending > up.
// A failed critical check means "down"; a failed non-critical one "degraded".
func Aggregate(results []Result) (string, Summary) {
	rank := map[string]int{StatusUp: 0, StatusPending: 1, StatusDegraded: 2, StatusDown: 3}
	sum := Summary{Total: len(results)}
	status := StatusUp
	raise := func(s string) {
		if rank[s] > rank[status] {
			status = s
		}
	}
	for _, res := range results {
		switch res.Status {
		case StatusUp:
			sum.Up++
		case StatusDown:
			sum.Down++
			if res.Critical {
				raise(StatusDown)
			} else {
				raise(StatusDegraded)
			}
		default:
			sum.Pending++
			raise(StatusPending)
		}
	}
	return status, sum
}
