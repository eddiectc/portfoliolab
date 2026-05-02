package portfolio

import "time"

// Portfolio represents a logical grouping of investment accounts.
type Portfolio struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateRequest is the DTO for creating a portfolio.
type CreateRequest struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// UpdateRequest is the DTO for updating a portfolio.
type UpdateRequest struct {
	Name     *string `json:"name,omitempty"`
	Currency *string `json:"currency,omitempty"`
}
