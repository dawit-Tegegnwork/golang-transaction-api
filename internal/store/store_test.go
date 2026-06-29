package store_test

import (
	"context"
	"testing"

	"github.com/dawit/golang-transaction-api/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.OpenSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	st := store.NewSQLite(db)
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

func seedUserAndAccount(t *testing.T, st *store.Store) (userID, accountID string) {
	t.Helper()
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "demo@example.com", "Demo User")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	acct, err := st.CreateAccount(ctx, user.ID, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return user.ID, acct.ID
}

func TestDeposit(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_, accountID := seedUserAndAccount(t, st)

	tx, err := st.Deposit(ctx, accountID, 5000, "seed")
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
	if tx.Amount != 5000 || tx.BalanceAfter != 5000 {
		t.Fatalf("unexpected tx: %+v", tx)
	}

	acct, err := st.GetAccount(ctx, accountID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acct.Balance != 5000 {
		t.Fatalf("balance = %d, want 5000", acct.Balance)
	}
}

func TestWithdraw(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_, accountID := seedUserAndAccount(t, st)

	if _, err := st.Deposit(ctx, accountID, 10000, ""); err != nil {
		t.Fatalf("deposit: %v", err)
	}
	tx, err := st.Withdraw(ctx, accountID, 3000, "atm")
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if tx.BalanceAfter != 7000 {
		t.Fatalf("balance_after = %d, want 7000", tx.BalanceAfter)
	}
}

func TestTransfer(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	userID, fromID := seedUserAndAccount(t, st)
	toAcct, err := st.CreateAccount(ctx, userID, "USD")
	if err != nil {
		t.Fatalf("create second account: %v", err)
	}

	if _, err := st.Deposit(ctx, fromID, 20000, ""); err != nil {
		t.Fatalf("deposit: %v", err)
	}
	txs, err := st.Transfer(ctx, fromID, toAcct.ID, 7500, "rent")
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if len(txs) != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", len(txs))
	}

	from, err := st.GetAccount(ctx, fromID)
	if err != nil {
		t.Fatalf("get from: %v", err)
	}
	to, err := st.GetAccount(ctx, toAcct.ID)
	if err != nil {
		t.Fatalf("get to: %v", err)
	}
	if from.Balance != 12500 || to.Balance != 7500 {
		t.Fatalf("balances from=%d to=%d", from.Balance, to.Balance)
	}
}

func TestInsufficientFunds(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_, accountID := seedUserAndAccount(t, st)

	if _, err := st.Deposit(ctx, accountID, 1000, ""); err != nil {
		t.Fatalf("deposit: %v", err)
	}
	_, err := st.Withdraw(ctx, accountID, 5000, "")
	if err != store.ErrInsufficientFunds {
		t.Fatalf("withdraw err = %v, want ErrInsufficientFunds", err)
	}

	user, err := st.CreateUser(ctx, "other@example.com", "Other User")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	fromAcct, err := st.CreateAccount(ctx, user.ID, "USD")
	if err != nil {
		t.Fatalf("create from account: %v", err)
	}
	toAcct, err := st.CreateAccount(ctx, user.ID, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	_, err = st.Transfer(ctx, fromAcct.ID, toAcct.ID, 100, "")
	if err != store.ErrInsufficientFunds {
		t.Fatalf("transfer err = %v, want ErrInsufficientFunds", err)
	}
}

func TestIdempotentRetry(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_, accountID := seedUserAndAccount(t, st)

	key := "idem-001"
	path := "/accounts/" + accountID + "/deposit"
	body := []byte(`{"id":"tx-1","amount":1000}`)

	if err := st.SaveIdempotency(ctx, key, path, 200, body); err != nil {
		t.Fatalf("save idempotency: %v", err)
	}
	status, cached, found, err := st.GetIdempotency(ctx, key, path)
	if err != nil {
		t.Fatalf("get idempotency: %v", err)
	}
	if !found || status != 200 || string(cached) != string(body) {
		t.Fatalf("unexpected cached response: found=%v status=%d body=%s", found, status, cached)
	}
}
