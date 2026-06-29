package models

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Account struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Currency  string    `json:"currency"`
	Balance   int64     `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
}

type Transaction struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"account_id"`
	CounterpartyID *string   `json:"counterparty_id,omitempty"`
	Type           string    `json:"type"`
	Amount         int64     `json:"amount"`
	BalanceAfter   int64     `json:"balance_after"`
	Reference      string    `json:"reference,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type DepositRequest struct {
	Amount    int64  `json:"amount"`
	Reference string `json:"reference,omitempty"`
}

type WithdrawRequest struct {
	Amount    int64  `json:"amount"`
	Reference string `json:"reference,omitempty"`
}

type TransferRequest struct {
	ToAccountID string `json:"to_account_id"`
	Amount      int64  `json:"amount"`
	Reference   string `json:"reference,omitempty"`
}

type CreateUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type CreateAccountRequest struct {
	Currency string `json:"currency"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
