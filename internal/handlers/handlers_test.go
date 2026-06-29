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
