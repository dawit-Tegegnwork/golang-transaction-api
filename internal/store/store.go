package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dawit/golang-transaction-api/internal/models"
	"github.com/google/uuid"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("amount must be positive")
	ErrSameAccount       = errors.New("cannot transfer to the same account")
)

type Store struct {
	db       *sql.DB
	postgres bool
}

func NewSQLite(db *sql.DB) *Store {
	return &Store{db: db, postgres: false}
}

func NewPostgres(db *sql.DB) *Store {
	return &Store{db: db, postgres: true}
}

func (s *Store) placeholder(n int) string {
	if s.postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

func (s *Store) Migrate(ctx context.Context) error {
	blobType := "BLOB"
	if s.postgres {
		blobType = "BYTEA"
	}
	schema := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    currency TEXT NOT NULL DEFAULT 'USD',
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transactions (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    counterparty_id TEXT REFERENCES accounts(id),
    type TEXT NOT NULL CHECK (type IN ('deposit', 'withdraw', 'transfer_in', 'transfer_out')),
    amount BIGINT NOT NULL CHECK (amount > 0),
    balance_after BIGINT NOT NULL,
    reference TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key TEXT NOT NULL,
    path TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    response_body %s NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (key, path)
);

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    details TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_transactions_account_id ON transactions(account_id);
CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
`, blobType)
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) CreateUser(ctx context.Context, email, name string) (*models.User, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	q := fmt.Sprintf(
		`INSERT INTO users (id, email, name, created_at) VALUES (%s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4),
	)
	_, err := s.db.ExecContext(ctx, q, id, email, name, now)
	if err != nil {
		return nil, err
	}
	user := &models.User{ID: id, Email: email, Name: name, CreatedAt: now}
	if err := s.audit(ctx, "system", "user.create", id, fmt.Sprintf("email=%s", email)); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Store) GetUser(ctx context.Context, id string) (*models.User, error) {
	q := fmt.Sprintf(`SELECT id, email, name, created_at FROM users WHERE id = %s`, s.placeholder(1))
	row := s.db.QueryRowContext(ctx, q, id)
	var u models.User
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) CreateAccount(ctx context.Context, userID, currency string) (*models.Account, error) {
	if _, err := s.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	if currency == "" {
		currency = "USD"
	}
	id := uuid.New().String()
	now := time.Now().UTC()
	q := fmt.Sprintf(
		`INSERT INTO accounts (id, user_id, currency, balance, created_at) VALUES (%s, %s, %s, 0, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4),
	)
	_, err := s.db.ExecContext(ctx, q, id, userID, currency, now)
	if err != nil {
		return nil, err
	}
	acct := &models.Account{ID: id, UserID: userID, Currency: currency, Balance: 0, CreatedAt: now}
	if err := s.audit(ctx, userID, "account.create", id, fmt.Sprintf("currency=%s", currency)); err != nil {
		return nil, err
	}
	return acct, nil
}

func (s *Store) GetAccount(ctx context.Context, id string) (*models.Account, error) {
	q := fmt.Sprintf(`SELECT id, user_id, currency, balance, created_at FROM accounts WHERE id = %s`, s.placeholder(1))
	row := s.db.QueryRowContext(ctx, q, id)
	var a models.Account
	if err := row.Scan(&a.ID, &a.UserID, &a.Currency, &a.Balance, &a.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (s *Store) ListTransactions(ctx context.Context, accountID string) ([]models.Transaction, error) {
	if _, err := s.GetAccount(ctx, accountID); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(
		`SELECT id, account_id, counterparty_id, type, amount, balance_after, COALESCE(reference, ''), created_at
		 FROM transactions WHERE account_id = %s ORDER BY created_at DESC`,
		s.placeholder(1),
	)
	rows, err := s.db.QueryContext(ctx, q, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []models.Transaction
	for rows.Next() {
		var t models.Transaction
		var counterparty sql.NullString
		if err := rows.Scan(&t.ID, &t.AccountID, &counterparty, &t.Type, &t.Amount, &t.BalanceAfter, &t.Reference, &t.CreatedAt); err != nil {
			return nil, err
		}
		if counterparty.Valid {
			t.CounterpartyID = &counterparty.String
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

func (s *Store) Deposit(ctx context.Context, accountID string, amount int64, reference string) (*models.Transaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	balance, err := s.lockAccount(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	newBalance := balance + amount
	if err := s.setBalance(ctx, tx, accountID, newBalance); err != nil {
		return nil, err
	}
	tr, err := s.insertTransaction(ctx, tx, accountID, nil, "deposit", amount, newBalance, reference)
	if err != nil {
		return nil, err
	}
	if err := s.auditTx(ctx, tx, "system", "account.deposit", accountID, fmt.Sprintf("amount=%d balance_after=%d", amount, newBalance)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tr, nil
}

func (s *Store) Withdraw(ctx context.Context, accountID string, amount int64, reference string) (*models.Transaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	balance, err := s.lockAccount(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	if balance < amount {
		return nil, ErrInsufficientFunds
	}
	newBalance := balance - amount
	if err := s.setBalance(ctx, tx, accountID, newBalance); err != nil {
		return nil, err
	}
	tr, err := s.insertTransaction(ctx, tx, accountID, nil, "withdraw", amount, newBalance, reference)
	if err != nil {
		return nil, err
	}
	if err := s.auditTx(ctx, tx, "system", "account.withdraw", accountID, fmt.Sprintf("amount=%d balance_after=%d", amount, newBalance)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tr, nil
}

func (s *Store) Transfer(ctx context.Context, fromID, toID string, amount int64, reference string) ([]models.Transaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	if fromID == toID {
		return nil, ErrSameAccount
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Lock both accounts in deterministic order to avoid deadlocks.
	first, second := fromID, toID
	if strings.Compare(fromID, toID) > 0 {
		first, second = toID, fromID
	}

	fromBalance, err := s.lockAccountByID(ctx, tx, first)
	if err != nil {
		return nil, err
	}
	toBalance, err := s.lockAccountByID(ctx, tx, second)
	if err != nil {
		return nil, err
	}

	var srcBalance, dstBalance int64
	if fromID == first {
		srcBalance = fromBalance
		dstBalance = toBalance
	} else {
		srcBalance = toBalance
		dstBalance = fromBalance
	}

	if srcBalance < amount {
		return nil, ErrInsufficientFunds
	}

	srcAccount, err := s.getAccountTx(ctx, tx, fromID)
	if err != nil {
		return nil, err
	}
	dstAccount, err := s.getAccountTx(ctx, tx, toID)
	if err != nil {
		return nil, err
	}
	if srcAccount.Currency != dstAccount.Currency {
		return nil, fmt.Errorf("currency mismatch")
	}

	newSrcBalance := srcBalance - amount
	newDstBalance := dstBalance + amount

	if err := s.setBalance(ctx, tx, fromID, newSrcBalance); err != nil {
		return nil, err
	}
	if err := s.setBalance(ctx, tx, toID, newDstBalance); err != nil {
		return nil, err
	}

	outTx, err := s.insertTransaction(ctx, tx, fromID, &toID, "transfer_out", amount, newSrcBalance, reference)
	if err != nil {
		return nil, err
	}
	inTx, err := s.insertTransaction(ctx, tx, toID, &fromID, "transfer_in", amount, newDstBalance, reference)
	if err != nil {
		return nil, err
	}
	if err := s.auditTx(ctx, tx, "system", "account.transfer", fromID,
		fmt.Sprintf("to=%s amount=%d src_balance_after=%d dst_balance_after=%d", toID, amount, newSrcBalance, newDstBalance)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return []models.Transaction{*outTx, *inTx}, nil
}

func (s *Store) GetIdempotency(ctx context.Context, key, path string) (status int, body []byte, found bool, err error) {
	q := fmt.Sprintf(
		`SELECT status_code, response_body FROM idempotency_keys WHERE key = %s AND path = %s`,
		s.placeholder(1), s.placeholder(2),
	)
	row := s.db.QueryRowContext(ctx, q, key, path)
	if err := row.Scan(&status, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil, false, nil
		}
		return 0, nil, false, err
	}
	return status, body, true, nil
}

func (s *Store) SaveIdempotency(ctx context.Context, key, path string, status int, body []byte) error {
	q := fmt.Sprintf(
		`INSERT INTO idempotency_keys (key, path, status_code, response_body, created_at) VALUES (%s, %s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5),
	)
	_, err := s.db.ExecContext(ctx, q, key, path, status, body, time.Now().UTC())
	return err
}

func (s *Store) lockAccount(ctx context.Context, tx *sql.Tx, accountID string) (int64, error) {
	return s.lockAccountByID(ctx, tx, accountID)
}

func (s *Store) lockAccountByID(ctx context.Context, tx *sql.Tx, accountID string) (int64, error) {
	lockClause := ""
	if s.postgres {
		lockClause = " FOR UPDATE"
	}
	q := fmt.Sprintf(`SELECT balance FROM accounts WHERE id = %s%s`, s.placeholder(1), lockClause)
	var balance int64
	err := tx.QueryRowContext(ctx, q, accountID).Scan(&balance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return balance, nil
}

func (s *Store) getAccountTx(ctx context.Context, tx *sql.Tx, accountID string) (*models.Account, error) {
	q := fmt.Sprintf(`SELECT id, user_id, currency, balance, created_at FROM accounts WHERE id = %s`, s.placeholder(1))
	row := tx.QueryRowContext(ctx, q, accountID)
	var a models.Account
	if err := row.Scan(&a.ID, &a.UserID, &a.Currency, &a.Balance, &a.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (s *Store) setBalance(ctx context.Context, tx *sql.Tx, accountID string, balance int64) error {
	q := fmt.Sprintf(`UPDATE accounts SET balance = %s WHERE id = %s`, s.placeholder(1), s.placeholder(2))
	res, err := tx.ExecContext(ctx, q, balance, accountID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) insertTransaction(ctx context.Context, tx *sql.Tx, accountID string, counterparty *string, typ string, amount, balanceAfter int64, reference string) (*models.Transaction, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	q := fmt.Sprintf(
		`INSERT INTO transactions (id, account_id, counterparty_id, type, amount, balance_after, reference, created_at)
		 VALUES (%s, %s, %s, %s, %s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4),
		s.placeholder(5), s.placeholder(6), s.placeholder(7), s.placeholder(8),
	)
	var cp interface{}
	if counterparty != nil {
		cp = *counterparty
	}
	_, err := tx.ExecContext(ctx, q, id, accountID, cp, typ, amount, balanceAfter, reference, now)
	if err != nil {
		return nil, err
	}
	return &models.Transaction{
		ID:             id,
		AccountID:      accountID,
		CounterpartyID: counterparty,
		Type:           typ,
		Amount:         amount,
		BalanceAfter:   balanceAfter,
		Reference:      reference,
		CreatedAt:      now,
	}, nil
}

func (s *Store) audit(ctx context.Context, actor, action, resource, details string) error {
	q := fmt.Sprintf(
		`INSERT INTO audit_log (id, actor, action, resource, details, created_at) VALUES (%s, %s, %s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6),
	)
	_, err := s.db.ExecContext(ctx, q, uuid.New().String(), actor, action, resource, details, time.Now().UTC())
	return err
}

func (s *Store) auditTx(ctx context.Context, tx *sql.Tx, actor, action, resource, details string) error {
	q := fmt.Sprintf(
		`INSERT INTO audit_log (id, actor, action, resource, details, created_at) VALUES (%s, %s, %s, %s, %s, %s)`,
		s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6),
	)
	_, err := tx.ExecContext(ctx, q, uuid.New().String(), actor, action, resource, details, time.Now().UTC())
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]models.AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	q := fmt.Sprintf(
		`SELECT id, actor, action, resource, COALESCE(details, ''), created_at FROM audit_log ORDER BY created_at DESC LIMIT %s`,
		s.placeholder(1),
	)
	rows, err := s.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []models.AuditEntry
	for rows.Next() {
		var e models.AuditEntry
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Resource, &e.Details, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) SeedDemo(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	user, err := s.CreateUser(ctx, "demo@portfolio.test", "Demo User")
	if err != nil {
		return err
	}
	acct, err := s.CreateAccount(ctx, user.ID, "USD")
	if err != nil {
		return err
	}
	_, err = s.Deposit(ctx, acct.ID, 10000, "demo-seed")
	return err
}
