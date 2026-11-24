// Package paging clamps the page/limit pair shared by every list endpoint.
package paging

const (
	// DefaultLimit applies when the caller omits limit or passes a non-positive one.
	DefaultLimit = 20
	// MaxLimit caps how many rows one response may carry.
	MaxLimit = 200
)

// Normalise clamps page and limit to the ranges the list endpoints use.
//
// These eight lines existed verbatim in internal/node, internal/source and
// internal/api/handler. All three agreed, which is exactly why nothing would ever
// flag them: the next change to a page size would land in one copy and the
// endpoints would quietly start paginating differently.
func Normalise(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	return page, limit
}
