package model

import "time"

const (
	KindHTML = "html"
	KindPSP  = "psp"

	ValuePrice = "price"
	ValueText  = "text"

	RuleAnyChange = "any_change"
	RuleDecrease  = "decrease"
	RuleBelow     = "below"
)

type Watch struct {
	ID                int64
	Name              string
	URL               string
	Kind              string
	Selector          string
	Attribute         string
	ValueType         string
	Currency          string
	AdapterConfig     string
	Rule              string
	ThresholdMinor    *int64
	Enabled           bool
	Revision          int64
	LastValue         string
	LastDisplay       string
	LastSuccessAt     *time.Time
	LastCheckAt       *time.Time
	LastError         string
	ConsecutiveErrors int
	ErrorAlerted      bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Observation struct {
	Normalized string
	Display    string
	Currency   string
	Raw        string
	Metadata   map[string]string
}

type Change struct {
	ID         int64
	WatchID    int64
	WatchName  string
	OldDisplay string
	NewDisplay string
	ObservedAt time.Time
}

type RunSummary struct {
	Checked int
	Changed int
	Errors  int
	Skipped bool
}
