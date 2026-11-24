package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Pagination holds paging metadata returned with list responses.
type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Paginated wraps a data slice with pagination metadata.
type Paginated struct {
	Data       any        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// NewPagination computes pagination metadata from raw counts.
func NewPagination(page, limit int, total int64) Pagination {
	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}
	return Pagination{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}

// OK sends HTTP 200 with the given payload.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// OKPaginated sends HTTP 200 with data + pagination wrapper.
func OKPaginated(c *gin.Context, data any, p Pagination) {
	c.JSON(http.StatusOK, Paginated{Data: data, Pagination: p})
}

// Created sends HTTP 201 with the created resource.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

// NoContent sends HTTP 204.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// BadRequest sends HTTP 400 with an error message.
func BadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": msg})
}

// NotFound sends HTTP 404 with an error message.
func NotFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, gin.H{"error": msg})
}

// Conflict sends HTTP 409 with an error message.
func Conflict(c *gin.Context, msg string) {
	c.JSON(http.StatusConflict, gin.H{"error": msg})
}

// InternalError sends HTTP 500 with a generic error message.
func InternalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}

// NotImplemented sends HTTP 501.
func NotImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// MaxPage bounds the page number.
//
// Without a bound, a page large enough makes (page-1)*limit overflow into a
// negative index, and callers slice with it: matched[start:end] with a negative
// start panics, which the recovery middleware turns into a 500 the caller can
// trigger on demand.
const MaxPage = 1_000_000

// ParsePage extracts and validates page/limit query params.
// Defaults: page=1, limit=20. Max limit=200, max page=MaxPage.
func ParsePage(c *gin.Context) (page, limit int) {
	page = 1
	limit = 20
	if v := c.Query("page"); v != "" {
		if n := parseInt(v); n > 0 {
			page = n
		}
	}
	if v := c.Query("limit"); v != "" {
		if n := parseInt(v); n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	if page > MaxPage {
		page = MaxPage
	}
	return page, limit
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		// Stop before n*10 can overflow: a long enough digit string would wrap,
		// and the wrapped value is then either rejected by n > 0 or, worse,
		// accepted as a plausible-looking page number.
		if n > (MaxPage-int(c-'0'))/10 {
			return MaxPage
		}
		n = n*10 + int(c-'0')
	}
	return n
}
