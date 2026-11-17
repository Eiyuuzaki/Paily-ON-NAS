package scoring

import "github.com/Eiyuuzaki/paily-on-nas/internal/db/model"

// sourceDirect marks a row the checker itself produced for this node.
const sourceDirect = "direct"

// filterDirect keeps only rows that describe this node's own measurement.
// Borrowed or inferred rows must never feed score, grade or freshness.
func filterDirect(recs []model.NodeDeepCheck) []model.NodeDeepCheck {
	out := recs[:0:0]
	for _, r := range recs {
		if r.Source == sourceDirect {
			out = append(out, r)
		}
	}
	return out
}
