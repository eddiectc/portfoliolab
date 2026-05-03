package account

import "time"

// Account represents a named investment account belonging to a portfolio.
type Account struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	PortfolioID int64     `json:"portfolio_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateRequest is the DTO for creating an account.
type CreateRequest struct {
	Name        string `json:"name"`
	PortfolioID int64  `json:"portfolio_id"`
}

// UpdateRequest is the DTO for updating an account.
type UpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	PortfolioID *int64  `json:"portfolio_id,omitempty"`
}
