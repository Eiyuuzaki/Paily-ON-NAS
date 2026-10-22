// Package filter provides the expr-lang expression evaluation engine used by
// the filtering and naming subsystems.
package filter

// SourceInfo is a condensed view of a source used inside node filter expressions.
type SourceInfo struct {
	ID         string
	Type       string // "subscribe" | "node"
	Identifier string
	Info       string
	Status     string // "active" | "dead"
	Content    string
	Priority   int    // 0=公益, 1=机场/付费, 2=最高
}

// NodeFilterEnv is the evaluation context for node filter expressions.
// Fields are flat (not nested) so expressions are written as e.g. Region == "HK".
type NodeFilterEnv struct {
	Server    string
	Protocol  string
	Password  string
	Hash      string
	Region    string
	Alive     bool
	Score     float64
	Latency   int // latest initial-check latency in ms; -1 = unreachable
	SourceIDs []string
	Sources   []SourceInfo   // full source details for this node
	RawConfig map[string]any // parsed clash proxy config map
	Streaming map[string]bool
	// SourcePriority is the max priority across all sources for this node.
	SourcePriority int
}

// SourceFilterEnv is the evaluation context for source filter expressions.
type SourceFilterEnv struct {
	ID         string
	Identifier string
	Info       string
	Status     string // "active" | "dead"
	Type       string // "subscribe" | "node"
}
