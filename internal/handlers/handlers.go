package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/dawit/golang-transaction-api/internal/models"
	"github.com/dawit/golang-transaction-api/internal/store"
)

const idempotencyHeader = "Idempotency-Key"

type API struct {
	Store *store.Store
}

func NewAPI(s *store.Store) *API {
	return &API{Store: s}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.landing)
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /audit", a.listAudit)
	mux.HandleFunc("POST /users", a.createUser)
	mux.HandleFunc("GET /users/{id}", a.getUser)
	mux.HandleFunc("POST /users/{id}/accounts", a.createAccount)
	mux.HandleFunc("GET /accounts/{id}", a.getAccount)
	mux.HandleFunc("POST /accounts/{id}/deposit", a.deposit)
	mux.HandleFunc("POST /accounts/{id}/withdraw", a.withdraw)
	mux.HandleFunc("POST /accounts/{id}/transfer", a.transfer)
	mux.HandleFunc("GET /accounts/{id}/transactions", a.listTransactions)
	return mux
}

func (a *API) landing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Golang Transaction API</title>
<style>body{font-family:system-ui,sans-serif;max-width:760px;margin:2rem auto;padding:0 1rem;line-height:1.6}
code{background:#f3f4f6;padding:.15rem .35rem;border-radius:4px}</style></head>
<body>
<h1>Golang Transaction API</h1>
<p>Portfolio reference implementation for wallet-style transfers with idempotency keys and PostgreSQL.</p>
<p><a href="/health">Health check</a> · Synthetic demo data only.</p>
<h2>Quick curl</h2>
<pre><code>curl http://localhost:8080/health
curl http://localhost:8080/audit?limit=10</code></pre>
</body></html>`))
}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	entries, err := a.Store.ListAudit(r.Context(), limit)
	if err != nil {
		log.Printf("list audit: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load audit log")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.Store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, models.ErrorResponse{Error: "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Email == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "email and name are required")
		return
	}
	user, err := a.Store.CreateUser(r.Context(), req.Email, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (a *API) getUser(w http.ResponseWriter, r *http.Request) {
	user, err := a.Store.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *API) createAccount(w http.ResponseWriter, r *http.Request) {
	var req models.CreateAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acct, err := a.Store.CreateAccount(r.Context(), r.PathValue("id"), req.Currency)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, acct)
}

func (a *API) getAccount(w http.ResponseWriter, r *http.Request) {
	acct, err := a.Store.GetAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func (a *API) deposit(w http.ResponseWriter, r *http.Request) {
	a.withIdempotency(w, r, func(ctx context.Context) (int, any) {
		var req models.DepositRequest
		if err := decodeJSON(r, &req); err != nil {
			return http.StatusBadRequest, models.ErrorResponse{Error: err.Error()}
		}
		tx, err := a.Store.Deposit(ctx, r.PathValue("id"), req.Amount, req.Reference)
		if err != nil {
			return storeErrorStatus(err), models.ErrorResponse{Error: err.Error()}
		}
		return http.StatusOK, tx
	})
}

func (a *API) withdraw(w http.ResponseWriter, r *http.Request) {
	a.withIdempotency(w, r, func(ctx context.Context) (int, any) {
		var req models.WithdrawRequest
		if err := decodeJSON(r, &req); err != nil {
			return http.StatusBadRequest, models.ErrorResponse{Error: err.Error()}
		}
		tx, err := a.Store.Withdraw(ctx, r.PathValue("id"), req.Amount, req.Reference)
		if err != nil {
			return storeErrorStatus(err), models.ErrorResponse{Error: err.Error()}
		}
		return http.StatusOK, tx
	})
}

func (a *API) transfer(w http.ResponseWriter, r *http.Request) {
	a.withIdempotency(w, r, func(ctx context.Context) (int, any) {
		var req models.TransferRequest
		if err := decodeJSON(r, &req); err != nil {
			return http.StatusBadRequest, models.ErrorResponse{Error: err.Error()}
		}
		txs, err := a.Store.Transfer(ctx, r.PathValue("id"), req.ToAccountID, req.Amount, req.Reference)
		if err != nil {
			return storeErrorStatus(err), models.ErrorResponse{Error: err.Error()}
		}
		return http.StatusOK, txs
	})
}

func (a *API) listTransactions(w http.ResponseWriter, r *http.Request) {
	txs, err := a.Store.ListTransactions(r.Context(), r.PathValue("id"))
	if err != nil {
		handleStoreError(w, err)
		return
	}
	if txs == nil {
		txs = []models.Transaction{}
	}
	writeJSON(w, http.StatusOK, txs)
}

type handlerFn func(ctx context.Context) (status int, body any)

func (a *API) withIdempotency(w http.ResponseWriter, r *http.Request, fn handlerFn) {
	key := strings.TrimSpace(r.Header.Get(idempotencyHeader))
	path := r.URL.Path

	if key != "" {
		status, body, found, err := a.Store.GetIdempotency(r.Context(), key, path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if found {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Idempotent-Replay", "true")
			w.WriteHeader(status)
			_, _ = w.Write(body)
			return
		}
	}

	status, body := fn(r.Context())
	payload, err := json.Marshal(body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if key != "" && status != http.StatusBadRequest {
		if err := a.Store.SaveIdempotency(r.Context(), key, path, status, payload); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var buf bytes.Buffer
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain a single JSON object")
		}
		return err
	}
	_ = buf
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, models.ErrorResponse{Error: msg})
}

func handleStoreError(w http.ResponseWriter, err error) {
	status := storeErrorStatus(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		log.Printf("internal store error: %v", err)
		msg = "internal server error"
	}
	writeError(w, status, msg)
}

func storeErrorStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrInsufficientFunds):
		return http.StatusUnprocessableEntity
	case errors.Is(err, store.ErrInvalidAmount), errors.Is(err, store.ErrSameAccount):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
