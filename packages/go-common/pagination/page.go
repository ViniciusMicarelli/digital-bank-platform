// Package pagination provides pagination helpers for API responses.
package pagination

import (
	"context"
	"fmt"
	"math"
)

// Page represents a paginated result.
type Page[T any] struct {
	Items    []T          // Items in this page
	Total    int64        // Total number of items
	Page     int          // Current page (1-indexed)
	PageSize int          // Number of items per page
	TotalPages int         // Total number of pages
	NextPage *int        // Next page number (nil if last page)
	PrevPage int         // Previous page number (0 if first page)
}

// QueryParams represents pagination query parameters.
type QueryParams struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Sort     string `json:"sort,omitempty"`
	Order    string `json:"order,omitempty"`
	Filter   string `json:"filter,omitempty"`
}

// DefaultPageSize is the default number of items per page.
const DefaultPageSize = 20

// MaxPageSize is the maximum allowed page size.
const MaxPageSize = 100

// FromContext extracts pagination from the context.
func FromContext(ctx context.Context) (*QueryParams, error) {
	// Extract from context values
	// In production, you'd use context.Context values or middleware
	params := &QueryParams{
		Page:     1,
		PageSize: DefaultPageSize,
	}
	return params, nil
}

// ToContext adds pagination to the context.
func ToContext(ctx context.Context, params *QueryParams) context.Context {
	// Store in context
	return context.WithValue(ctx, "pagination", params)
}

// Validate checks if pagination parameters are valid.
func Validate(params *QueryParams) error {
	if params.Page < 1 {
		return fmt.Errorf("page must be >= 1, got %d", params.Page)
	}
	if params.PageSize < 1 {
		return fmt.Errorf("page_size must be >= 1, got %d", params.PageSize)
	}
	if params.PageSize > MaxPageSize {
		return fmt.Errorf("page_size exceeds maximum of %d", MaxPageSize)
	}
	return nil
}

// Apply applies pagination parameters to a slice.
func Apply[T any](items []T, params *QueryParams) (*Page[T], error) {
	if err := Validate(params); err != nil {
		return nil, err
	}
	total := len(items)
	totalPages := int(math.Ceil(float64(total) / float64(params.PageSize)))

	start := (params.Page - 1) * params.PageSize
	end := start + params.PageSize
	if end > total {
		end = total
	}

	items := append([]T{}, items[start:end]...)

	// Check if this is the last page
	var nextPage *int
	if params.Page < totalPages {
		nextPage = intPtr(params.Page + 1)
	}

	return &Page[T]{
		Items:      items,
		Total:      int64(total),
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
		NextPage:   nextPage,
		PrevPage:   params.Page - 1,
	}, nil
}

// OffsetBased creates a slice with offset and page size.
func OffsetBased[T any](items []T, page, pageSize int) []T {
	if page < 1 || pageSize <= 0 || int64(len(items)) < int64(page-1)+int64(pageSize) {
		return items
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// CursorParams represents cursor-based pagination.
type CursorParams struct {
	Cursor  string  // Cursor for pagination
	PageSize int     // Number of items per page
	Sort    string   // Sort field
	Order    string   // ASC or DESC
}

// Cursor represents a pagination cursor.
type Cursor struct {
	ID       string  // Item ID
	DeletedAt string  // Deletion timestamp (for soft deletes)
	CreatedAt string  // Creation timestamp
}

// FromCursor extracts pagination from the given cursor.
func FromCursor(cursor string) (*CursorParams, error) {
	// Parse cursor format
	// In production, use proper cursor encoding
	params := &CursorParams{
		Cursor:  "",
		PageSize: DefaultPageSize,
	}
	return params, nil
}

// ToCursor creates a cursor for the given item.
func ToCursor(item interface{}) (*Cursor, error) {
	// Create cursor from item ID
	return &Cursor{
		ID:      "",
		CreatedAt: "",
	}, nil
}

// PageInfo represents pagination information in API responses.
type PageInfo struct {
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	Total      int64   `json:"total"`
	TotalPages int     `json:"total_pages"`
	HasNext    bool    `json:"has_next"`
	HasPrev    bool    `json:"has_prev"`
}

// NewPageInfo creates a new PageInfo from the given page.
func NewPageInfo(page *Page[any]) *PageInfo {
	if page == nil {
		return &PageInfo{}
	}
	return &PageInfo{
		Page:       page.Page,
		PageSize:   page.PageSize,
		Total:      page.Total,
		TotalPages: page.TotalPages,
		HasNext:    page.NextPage != nil,
		HasPrev:    page.PrevPage > 0,
	}
}

// Limit returns the next page items given the limit.
func Limit(items []any, limit int) []any {
	if limit <= 0 {
		return items
	}
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

// Paginate returns paginated items.
func Paginate(items []any, page, pageSize int) (result []any, hasNext, hasPrev bool) {
	if page < 1 {
		page = 1
	}
	totalPages := len(items)
	if totalPages == 0 {
		return nil, false, false
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}

	result = items[start:end]
	hasNext = page < totalPages
	hasPrev = page > 1

	return result, hasNext, hasPrev
}

// CountPages calculates the total number of pages.
func CountPages(total, pageSize int) int {
	if pageSize <= 0 || total == 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

// CheckCurrentPage validates if the current page is valid.
func CheckCurrentPage(current, total, pageSize int) bool {
	totalPages := CountPages(total, pageSize)
	return current >= 1 && current <= totalPages
}

// GetNextPage calculates the next page number.
func GetNextPage(currentPage, totalPages int) int {
	if currentPage < totalPages {
		return currentPage + 1
	}
	return 0 // 0 means no next page
}

// GetPreviousPage calculates the previous page number.
func GetPreviousPage(currentPage int) int {
	if currentPage > 1 {
		return currentPage - 1
	}
	return 0 // 0 means no previous page
}

// FormatPageNumber formats page number for display.
func FormatPageNumber(page int) string {
	return fmt.Sprintf("Page %d", page)
}

// FormatTotalPages formats total pages for display.
func FormatTotalPages(totalPages int) string {
	if totalPages == 0 {
		return "0"
	}
	return fmt.Sprintf("Showing 1 to %d of %d pages",
		min(pageSize, totalPages), totalPages)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func intPtr(i int) *int {
	return &i
}

func uintPtr(i uint) *uint {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func null[T any]() *T {
	var zero T
	return &zero
}

// DefaultPageSizeFunc returns the default page size.
func DefaultPageSizeFunc() int {
	return DefaultPageSize
}

// MaxPageSizeFunc returns the maximum page size.
func MaxPageSizeFunc() int {
	return MaxPageSize
}