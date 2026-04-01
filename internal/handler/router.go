package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"cc2api/internal/config"
	"cc2api/internal/middleware"
	"cc2api/internal/model"
	"cc2api/internal/service"
)

type Router struct {
	mux        *http.ServeMux
	cfg        *config.Config
	gatewaySvc *service.GatewayService
	accountSvc *service.AccountService
	usageSvc   *service.UsageService
	tokenTester *service.TokenTester
}

func NewRouter(cfg *config.Config, gw *service.GatewayService, acc *service.AccountService, usage *service.UsageService, tt *service.TokenTester) *Router {
	r := &Router{
		mux:         http.NewServeMux(),
		cfg:         cfg,
		gatewaySvc:  gw,
		accountSvc:  acc,
		usageSvc:    usage,
		tokenTester: tt,
	}
	r.setup()
	return r
}

func (r *Router) Run(addr string) error {
	return http.ListenAndServe(addr, r.mux)
}

func (r *Router) setup() {
	gatewayAuth := middleware.Auth(r.cfg.Admin.APIKey)
	adminAuth := middleware.Auth(r.cfg.Admin.Password)

	// Models endpoint (API key auth)
	r.mux.Handle("GET /v1/models", gatewayAuth(http.HandlerFunc(r.listModels)))

	// Gateway endpoints (API key auth)
	r.mux.Handle("/v1/", gatewayAuth(http.HandlerFunc(r.gatewaySvc.HandleRequest)))
	r.mux.Handle("/api/", gatewayAuth(http.HandlerFunc(r.gatewaySvc.HandleRequest)))

	// Admin API (password auth)
	r.mux.Handle("GET /admin/accounts", adminAuth(http.HandlerFunc(r.listAccounts)))
	r.mux.Handle("POST /admin/accounts", adminAuth(http.HandlerFunc(r.createAccount)))
	r.mux.Handle("PUT /admin/accounts/{id}", adminAuth(http.HandlerFunc(r.updateAccount)))
	r.mux.Handle("DELETE /admin/accounts/{id}", adminAuth(http.HandlerFunc(r.deleteAccount)))
	r.mux.Handle("POST /admin/accounts/{id}/test", adminAuth(http.HandlerFunc(r.testAccount)))
	r.mux.Handle("GET /admin/usage", adminAuth(http.HandlerFunc(r.getUsage)))
	r.mux.Handle("GET /admin/dashboard", adminAuth(http.HandlerFunc(r.getDashboard)))

	// Health
	r.mux.HandleFunc("GET /_health", r.health)

	// Frontend: serve web/dist/ with SPA fallback to index.html
	if info, err := os.Stat("web/dist/index.html"); err == nil && !info.IsDir() {
		fileServer := http.FileServer(http.Dir("web/dist"))
		r.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			// Try static file first
			path := "web/dist" + req.URL.Path
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, req)
				return
			}
			// SPA fallback: serve index.html
			http.ServeFile(w, req, "web/dist/index.html")
		})
	}
}

func (r *Router) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (r *Router) listAccounts(w http.ResponseWriter, req *http.Request) {
	accounts, err := r.accountSvc.ListAccounts(req.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if accounts == nil {
		accounts = []*model.Account{}
	}
	writeJSON(w, 200, accounts)
}

func (r *Router) createAccount(w http.ResponseWriter, req *http.Request) {
	var a model.Account
	if err := json.NewDecoder(req.Body).Decode(&a); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}
	if a.Token == "" || a.Email == "" {
		writeJSON(w, 400, map[string]string{"error": "token and email are required"})
		return
	}
	if err := r.accountSvc.CreateAccount(req.Context(), &a); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, a)
}

func (r *Router) updateAccount(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid id"})
		return
	}

	existing, err := r.accountSvc.GetAccount(req.Context(), id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "account not found"})
		return
	}

	var updates model.Account
	if err := json.NewDecoder(req.Body).Decode(&updates); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}

	// Merge updates
	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Email != "" {
		existing.Email = updates.Email
	}
	if updates.Token != "" {
		existing.Token = updates.Token
	}
	if updates.ProxyURL != "" {
		existing.ProxyURL = updates.ProxyURL
	}
	if updates.Concurrency > 0 {
		existing.Concurrency = updates.Concurrency
	}
	if updates.Priority > 0 {
		existing.Priority = updates.Priority
	}
	if updates.Status != "" {
		existing.Status = updates.Status
	}

	if err := r.accountSvc.UpdateAccount(req.Context(), existing); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, existing)
}

func (r *Router) deleteAccount(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid id"})
		return
	}
	if err := r.accountSvc.DeleteAccount(req.Context(), id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (r *Router) testAccount(w http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid id"})
		return
	}
	account, err := r.accountSvc.GetAccount(req.Context(), id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "account not found"})
		return
	}
	if err := r.tokenTester.TestToken(req.Context(), account.Token, account.ProxyURL); err != nil {
		writeJSON(w, 200, map[string]string{"status": "error", "message": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (r *Router) getUsage(w http.ResponseWriter, req *http.Request) {
	hours := 24
	if h := req.URL.Query().Get("hours"); h != "" {
		if parsed, err := strconv.Atoi(h); err == nil && parsed > 0 {
			hours = parsed
		}
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats, err := r.usageSvc.StatsByAccount(req.Context(), since)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if stats == nil {
		writeJSON(w, 200, []struct{}{})
		return
	}
	writeJSON(w, 200, stats)
}

func (r *Router) getDashboard(w http.ResponseWriter, req *http.Request) {
	accounts, err := r.accountSvc.ListAccounts(req.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	active, errCount, disabled := 0, 0, 0
	for _, a := range accounts {
		switch a.Status {
		case model.AccountStatusActive:
			active++
		case model.AccountStatusError:
			errCount++
		case model.AccountStatusDisabled:
			disabled++
		}
	}

	since := time.Now().Add(-24 * time.Hour)
	stats, _ := r.usageSvc.StatsByAccount(req.Context(), since)

	totalReqs, totalInput, totalOutput := 0, 0, 0
	for _, s := range stats {
		totalReqs += s.TotalRequests
		totalInput += s.TotalInputTokens
		totalOutput += s.TotalOutputTokens
	}

	writeJSON(w, 200, map[string]any{
		"accounts": map[string]int{
			"total": len(accounts), "active": active, "error": errCount, "disabled": disabled,
		},
		"usage_24h": map[string]int{
			"requests": totalReqs, "input_tokens": totalInput, "output_tokens": totalOutput,
		},
	})
}

var defaultModels = []map[string]string{
	{"id": "claude-opus-4-5-20251101", "type": "model", "display_name": "Claude Opus 4.5", "created_at": "2025-11-01T00:00:00Z"},
	{"id": "claude-opus-4-6", "type": "model", "display_name": "Claude Opus 4.6", "created_at": "2026-02-06T00:00:00Z"},
	{"id": "claude-sonnet-4-6", "type": "model", "display_name": "Claude Sonnet 4.6", "created_at": "2026-02-18T00:00:00Z"},
	{"id": "claude-sonnet-4-5-20250929", "type": "model", "display_name": "Claude Sonnet 4.5", "created_at": "2025-09-29T00:00:00Z"},
	{"id": "claude-haiku-4-5-20251001", "type": "model", "display_name": "Claude Haiku 4.5", "created_at": "2025-10-01T00:00:00Z"},
}

func (r *Router) listModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"data":   defaultModels,
		"object": "list",
	})
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
