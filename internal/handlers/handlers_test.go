package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dawit/golang-transaction-api/internal/handlers"
	"github.com/dawit/golang-transaction-api/internal/store"
)

func setupAPI(t *testing.T) (*handlers.API, string) {
	t.Helper()
	db, err := store.OpenSQLite(t.TempDir() + "/api.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	st := store.NewSQLite(db)
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "api@example.com", "API User")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	acct, err := st.CreateAccount(context.Background(), user.ID, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return handlers.NewAPI(st), acct.ID
}

func TestDepositIdempotentHTTP(t *testing.T) {
	api, accountID := setupAPI(t)
	srv := api.Routes()

	body := []byte(`{"amount":2500,"reference":"payroll"}`)
	req1 := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/deposit", bytes.NewReader(body))
	req1.Header.Set("Idempotency-Key", "http-idem-1")
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status = %d body=%s", rec1.Code, rec1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/deposit", bytes.NewReader(body))
	req2.Header.Set("Idempotency-Key", "http-idem-1")
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("replay status = %d", rec2.Code)
	}
	if rec2.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("expected idempotent replay header")
	}
	if rec1.Body.String() != rec2.Body.String() {
		t.Fatalf("responses differ:\n%s\n%s", rec1.Body.String(), rec2.Body.String())
	}

	var acct struct {
		Balance int64 `json:"balance"`
	}
	getReq := httptest.NewRequest(http.MethodGet, "/accounts/"+accountID, nil)
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)
	if err := json.Unmarshal(getRec.Body.Bytes(), &acct); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	if acct.Balance != 2500 {
		t.Fatalf("balance = %d, want 2500 (single deposit)", acct.Balance)
	}
}

func TestHealth(t *testing.T) {
	api, _ := setupAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d", rec.Code)
	}
}

func TestWithdrawHTTP(t *testing.T) {
	api, accountID := setupAPI(t)
	srv := api.Routes()

	deposit := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/deposit", bytes.NewReader([]byte(`{"amount":5000,"reference":"seed"}`)))
	depositRec := httptest.NewRecorder()
	srv.ServeHTTP(depositRec, deposit)
	if depositRec.Code != http.StatusOK {
		t.Fatalf("deposit status = %d", depositRec.Code)
	}

	withdraw := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/withdraw", bytes.NewReader([]byte(`{"amount":1500,"reference":"atm"}`)))
	withdrawRec := httptest.NewRecorder()
	srv.ServeHTTP(withdrawRec, withdraw)
	if withdrawRec.Code != http.StatusOK {
		t.Fatalf("withdraw status = %d body=%s", withdrawRec.Code, withdrawRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/accounts/"+accountID, nil)
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)
	var acct struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &acct); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	if acct.Balance != 3500 {
		t.Fatalf("balance = %d, want 3500", acct.Balance)
	}
}

func TestTransferHTTP(t *testing.T) {
	api, fromAccountID := setupAPI(t)
	srv := api.Routes()
	ctx := context.Background()

	userB, err := api.Store.CreateUser(ctx, "bob@demo.test", "Bob Demo")
	if err != nil {
		t.Fatalf("create user b: %v", err)
	}
	toAccount, err := api.Store.CreateAccount(ctx, userB.ID, "USD")
	if err != nil {
		t.Fatalf("create account b: %v", err)
	}

	deposit := httptest.NewRequest(http.MethodPost, "/accounts/"+fromAccountID+"/deposit", bytes.NewReader([]byte(`{"amount":4000,"reference":"seed"}`)))
	depositRec := httptest.NewRecorder()
	srv.ServeHTTP(depositRec, deposit)
	if depositRec.Code != http.StatusOK {
		t.Fatalf("deposit status = %d", depositRec.Code)
	}

	body := []byte(`{"to_account_id":"` + toAccount.ID + `","amount":1200,"reference":"rent"}`)
	transfer := httptest.NewRequest(http.MethodPost, "/accounts/"+fromAccountID+"/transfer", bytes.NewReader(body))
	transferRec := httptest.NewRecorder()
	srv.ServeHTTP(transferRec, transfer)
	if transferRec.Code != http.StatusOK {
		t.Fatalf("transfer status = %d body=%s", transferRec.Code, transferRec.Body.String())
	}

	fromReq := httptest.NewRequest(http.MethodGet, "/accounts/"+fromAccountID, nil)
	fromRec := httptest.NewRecorder()
	srv.ServeHTTP(fromRec, fromReq)
	var fromAcct struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(fromRec.Body.Bytes(), &fromAcct); err != nil {
		t.Fatalf("decode from account: %v", err)
	}
	if fromAcct.Balance != 2800 {
		t.Fatalf("from balance = %d, want 2800", fromAcct.Balance)
	}

	toReq := httptest.NewRequest(http.MethodGet, "/accounts/"+toAccount.ID, nil)
	toRec := httptest.NewRecorder()
	srv.ServeHTTP(toRec, toReq)
	var toAcct struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(toRec.Body.Bytes(), &toAcct); err != nil {
		t.Fatalf("decode to account: %v", err)
	}
	if toAcct.Balance != 1200 {
		t.Fatalf("to balance = %d, want 1200", toAcct.Balance)
	}
}

func TestWithdrawInsufficientFundsHTTP(t *testing.T) {
	api, accountID := setupAPI(t)
	srv := api.Routes()

	withdraw := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/withdraw", bytes.NewReader([]byte(`{"amount":100,"reference":"overdraft"}`)))
	withdrawRec := httptest.NewRecorder()
	srv.ServeHTTP(withdrawRec, withdraw)
	if withdrawRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", withdrawRec.Code, withdrawRec.Body.String())
	}
}
