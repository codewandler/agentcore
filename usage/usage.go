// Package usage tracks model token and cost usage across agent turns.
package usage

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/codewandler/llmadapter/unified"
)

type Dims struct {
	Provider       string            `json:"provider,omitempty"`
	Model          string            `json:"model,omitempty"`
	RequestID      string            `json:"request_id,omitempty"`
	TurnID         string            `json:"turn_id,omitempty"`
	SessionID      string            `json:"session_id,omitempty"`
	ConversationID string            `json:"conversation_id,omitempty"`
	BranchID       string            `json:"branch_id,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
}

type Record struct {
	Usage      unified.Usage  `json:"usage"`
	Dims       Dims           `json:"dims"`
	IsEstimate bool           `json:"is_estimate,omitempty"`
	RecordedAt time.Time      `json:"recorded_at"`
	Source     string         `json:"source,omitempty"`
	Encoder    string         `json:"encoder,omitempty"`
	Extras     map[string]any `json:"extras,omitempty"`
}

type CostCalculator interface {
	Calculate(provider, model string, tokens unified.TokenItems) (unified.CostItems, bool)
}

type CostCalculatorFunc func(provider, model string, tokens unified.TokenItems) (unified.CostItems, bool)

func (f CostCalculatorFunc) Calculate(p, m string, t unified.TokenItems) (unified.CostItems, bool) {
	return f(p, m, t)
}

type Budget interface {
	Exceeded(Record) bool
}

type Tracker struct {
	mu         sync.Mutex
	records    []Record
	budget     Budget
	calculator CostCalculator
}

type TrackerOption func(*Tracker)

func WithBudget(b Budget) TrackerOption {
	return func(t *Tracker) { t.budget = b }
}

func WithCostCalculator(c CostCalculator) TrackerOption {
	return func(t *Tracker) { t.calculator = c }
}

func NewTracker(opts ...TrackerOption) *Tracker {
	t := &Tracker{}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func FromUnified(u unified.Usage, dims Dims) Record {
	return Record{Usage: normalizeUsage(u), Dims: dims, RecordedAt: time.Now()}
}

func (t *Tracker) Record(r Record) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r.RecordedAt.IsZero() {
		r.RecordedAt = time.Now()
	}
	r.Usage = normalizeUsage(r.Usage)
	if len(r.Usage.Costs) == 0 && t.calculator != nil {
		if costs, ok := t.calculator.Calculate(r.Dims.Provider, r.Dims.Model, r.Usage.Tokens); ok {
			r.Usage.Costs = costs.NonZero()
		}
	}
	t.records = append(t.records, r)
}

func (t *Tracker) Records() []Record {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Record, len(t.records))
	copy(out, t.records)
	return out
}

func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = nil
}

func (t *Tracker) Aggregate() Record {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Merge(filterRecords(t.records, ExcludeEstimates())...)
}

func (t *Tracker) Filter(fs ...FilterFunc) []Record {
	t.mu.Lock()
	defer t.mu.Unlock()
	return filterRecords(t.records, fs...)
}

func (t *Tracker) WithinBudget() bool {
	if t.budget == nil {
		return true
	}
	return !t.budget.Exceeded(t.Aggregate())
}

func Merge(records ...Record) Record {
	var out Record
	out.RecordedAt = time.Now()
	tokenCounts := map[unified.TokenKind]int{}
	costAmounts := map[unified.CostKind]float64{}
	for _, r := range records {
		if out.RecordedAt.IsZero() || (!r.RecordedAt.IsZero() && r.RecordedAt.Before(out.RecordedAt)) {
			out.RecordedAt = r.RecordedAt
		}
		for _, item := range r.Usage.Tokens {
			tokenCounts[item.Kind] += item.Count
		}
		for _, item := range r.Usage.Costs {
			costAmounts[item.Kind] += item.Amount
		}
		if out.Dims.Provider == "" {
			out.Dims = r.Dims
		}
	}
	for kind, count := range tokenCounts {
		if count != 0 {
			out.Usage.Tokens = append(out.Usage.Tokens, unified.TokenItem{Kind: kind, Count: count})
		}
	}
	for kind, amount := range costAmounts {
		if amount != 0 {
			out.Usage.Costs = append(out.Usage.Costs, unified.CostItem{Kind: kind, Amount: amount})
		}
	}
	sort.Slice(out.Usage.Tokens, func(i, j int) bool {
		return out.Usage.Tokens[i].Kind < out.Usage.Tokens[j].Kind
	})
	sort.Slice(out.Usage.Costs, func(i, j int) bool {
		return out.Usage.Costs[i].Kind < out.Usage.Costs[j].Kind
	})
	return out
}

type Drift struct {
	Dims           Dims
	EstimatedInput int
	ActualInput    int
	InputDelta     int
	InputPct       float64
	Estimate       Record
	Actual         Record
}

type DriftStats struct {
	N       int
	MinPct  float64
	MaxPct  float64
	MeanPct float64
	P50Pct  float64
	P95Pct  float64
}

func ComputeDrift(estimate, actual *Record) *Drift {
	if estimate == nil || actual == nil || !estimate.IsEstimate {
		return nil
	}
	estInput := estimate.Usage.InputTokens()
	actInput := actual.Usage.InputTokens()
	delta := actInput - estInput
	pct := math.NaN()
	if estInput > 0 {
		pct = float64(delta) / float64(estInput) * 100
	}
	return &Drift{
		Dims:           actual.Dims,
		EstimatedInput: estInput,
		ActualInput:    actInput,
		InputDelta:     delta,
		InputPct:       pct,
		Estimate:       *estimate,
		Actual:         *actual,
	}
}

func (t *Tracker) Drift(requestID string) (*Drift, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var estimate, actual *Record
	for i := range t.records {
		r := &t.records[i]
		if r.Dims.RequestID != requestID {
			continue
		}
		if r.IsEstimate && estimate == nil {
			estimate = r
		}
		if !r.IsEstimate && actual == nil {
			actual = r
		}
	}
	if d := ComputeDrift(estimate, actual); d != nil {
		return d, true
	}
	return nil, false
}

func (t *Tracker) Drifts() []Drift {
	t.mu.Lock()
	defer t.mu.Unlock()
	pairs := map[string]struct {
		estimate *Record
		actual   *Record
	}{}
	for i := range t.records {
		r := &t.records[i]
		if r.Dims.RequestID == "" {
			continue
		}
		p := pairs[r.Dims.RequestID]
		if r.IsEstimate && p.estimate == nil {
			p.estimate = r
		}
		if !r.IsEstimate && p.actual == nil {
			p.actual = r
		}
		pairs[r.Dims.RequestID] = p
	}
	var out []Drift
	for _, p := range pairs {
		if d := ComputeDrift(p.estimate, p.actual); d != nil {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Actual.RecordedAt.Before(out[j].Actual.RecordedAt)
	})
	return out
}

func (t *Tracker) DriftStats() DriftStats {
	drifts := t.Drifts()
	if len(drifts) == 0 {
		return DriftStats{}
	}
	pcts := make([]float64, 0, len(drifts))
	var sum float64
	minPct, maxPct := math.MaxFloat64, -math.MaxFloat64
	for _, d := range drifts {
		if math.IsNaN(d.InputPct) {
			continue
		}
		pcts = append(pcts, d.InputPct)
		sum += d.InputPct
		if d.InputPct < minPct {
			minPct = d.InputPct
		}
		if d.InputPct > maxPct {
			maxPct = d.InputPct
		}
	}
	if len(pcts) == 0 {
		return DriftStats{N: len(drifts)}
	}
	sort.Float64s(pcts)
	p95 := int(float64(len(pcts)) * 0.95)
	if p95 >= len(pcts) {
		p95 = len(pcts) - 1
	}
	return DriftStats{
		N:       len(drifts),
		MinPct:  minPct,
		MaxPct:  maxPct,
		MeanPct: sum / float64(len(pcts)),
		P50Pct:  pcts[len(pcts)/2],
		P95Pct:  pcts[p95],
	}
}

type FilterFunc func(Record) bool

func ByProvider(name string) FilterFunc {
	return func(r Record) bool { return r.Dims.Provider == name }
}

func ByModel(model string) FilterFunc {
	return func(r Record) bool { return r.Dims.Model == model }
}

func ByTurnID(id string) FilterFunc {
	return func(r Record) bool { return r.Dims.TurnID == id }
}

func BySessionID(id string) FilterFunc {
	return func(r Record) bool { return r.Dims.SessionID == id }
}

func EstimatesOnly() FilterFunc {
	return func(r Record) bool { return r.IsEstimate }
}

func ExcludeEstimates() FilterFunc {
	return func(r Record) bool { return !r.IsEstimate }
}

func Since(t time.Time) FilterFunc {
	return func(r Record) bool { return r.RecordedAt.After(t) }
}

func ByLabel(key, value string) FilterFunc {
	return func(r Record) bool {
		return r.Dims.Labels != nil && r.Dims.Labels[key] == value
	}
}

func filterRecords(records []Record, fs ...FilterFunc) []Record {
	var out []Record
outer:
	for _, r := range records {
		for _, f := range fs {
			if f != nil && !f(r) {
				continue outer
			}
		}
		out = append(out, r)
	}
	return out
}

func normalizeUsage(u unified.Usage) unified.Usage {
	u.Tokens = u.Tokens.NonZero()
	u.Costs = u.Costs.NonZero()
	return u
}
