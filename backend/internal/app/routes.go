package app

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, map[string]any{"ok": true, "database": s.db != nil}) })

	mux.HandleFunc("GET /api/clusters", s.handleClusters)
	mux.HandleFunc("GET /api/triggers", s.handlePopularTriggers)
	mux.HandleFunc("GET /api/jokes", s.handleListJokes)
	mux.HandleFunc("GET /api/jokes/{id}", s.handleGetJoke)
	mux.HandleFunc("POST /api/jokes/analyze", s.handleAnalyzeJoke)
	mux.HandleFunc("POST /api/jokes", s.handleCreateJoke)
	mux.HandleFunc("POST /api/jokes/{id}/like", s.handleLike)
	mux.HandleFunc("POST /api/jokes/{id}/favorite", s.handleFavorite)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("GET /api/users/{id}", s.handleGetPublicUser)
	mux.HandleFunc("POST /api/onboarding/complete", s.handleOnboardingComplete)

	mux.HandleFunc("POST /api/standup/suggest-jokes", s.handleSuggestJokes)
	mux.HandleFunc("POST /api/standup/generate", s.handleGenerateMonologue)
	mux.HandleFunc("POST /api/payments/create", s.handleCreatePayment)
	mux.HandleFunc("POST /api/payments/webhook", s.handlePaymentWebhook)
	mux.HandleFunc("GET /api/payments/history", s.handlePaymentHistory)
	mux.HandleFunc("POST /api/admin/import-csv", s.handleCSVImport)
	mux.HandleFunc("GET /api/admin/imports/{id}", s.handleImportStatus)
	mux.HandleFunc("GET /api/admin/jokes", s.handleAdminJokes)
	mux.HandleFunc("PATCH /api/admin/jokes/{id}", s.handleAdminUpdateJoke)
	mux.HandleFunc("DELETE /api/admin/jokes/{id}", s.handleAdminDeleteJoke)
	mux.HandleFunc("GET /api/admin/payments", s.handleAdminPayments)

	mux.HandleFunc("GET /auth/yandex/login", s.yandexLogin)
	mux.HandleFunc("GET /auth/yandex/callback", s.yandexCallback)
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "jg_session", Value: "", Path: "/", HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	mux.HandleFunc("GET /robots.txt", s.handleRobots)
	mux.HandleFunc("GET /joke/{id}", s.handleSSRJoke)
	mux.HandleFunc("GET /cluster/{slug}", s.handleSSRCluster)
	mux.HandleFunc("GET /tag/{tag}", s.handleSSRTag)
	mux.HandleFunc("GET /user/me", s.handleSPA)
	mux.HandleFunc("GET /user/{id}", s.handleSSRUser)
	for _, path := range []string{"/{$}", "/standup", "/favorites", "/admin"} {
		mux.HandleFunc("GET "+path, s.handleSPA)
	}

	dist := s.frontendDist()
	assetsDir := filepath.Join(dist, "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))
	for _, name := range []string{"manifest.json", "sw.js", "icon.svg", "icon-192.png", "icon-512.png"} {
		filename := name
		mux.HandleFunc("GET /"+filename, func(w http.ResponseWriter, r *http.Request) {
			path := filepath.Join(dist, filename)
			if _, err := os.Stat(path); err != nil { http.NotFound(w, r); return }
			http.ServeFile(w, r, path)
		})
	}
	// A final not-found handler avoids serving the SPA for mistyped API paths.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") { writeJSON(w, http.StatusNotFound, map[string]string{"error": "Маршрут не найден."}); return }
		s.handleSPA(w, r)
	})
	return mux
}

func (s *Server) Handler() http.Handler {
	if s.mux == nil { s.mux = s.routes() }
	return s.mux
}

func (s *Server) frontendDist() string {
	if filepath.IsAbs(s.cfg.FrontendDist) { return s.cfg.FrontendDist }
	if _, err := os.Stat(s.cfg.FrontendDist); err == nil { return s.cfg.FrontendDist }
	cwd, err := os.Getwd()
	if err != nil { return s.cfg.FrontendDist }
	candidates := []string{filepath.Join(cwd, "frontend", "dist"), filepath.Join(cwd, "..", "frontend", "dist")}
	for _, candidate := range candidates { if _, err := os.Stat(candidate); err == nil { return candidate } }
	return s.cfg.FrontendDist
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet { http.Error(w, "Метод не поддерживается.", http.StatusMethodNotAllowed); return }
	index := filepath.Join(s.frontendDist(), "index.html")
	content, err := os.ReadFile(index)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!doctype html><html lang="ru"><meta charset="utf-8"><title>Галактика юмора</title><body style="font:16px sans-serif;background:#090b15;color:#eee;padding:4rem"><h1>Галактика юмора</h1><p>Соберите интерфейс командой <code>cd frontend &amp;&amp; npm install &amp;&amp; npm run build</code>, затем обновите страницу.</p></body></html>`))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(content)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Не удалось прочитать запрос."})
		return false
	}
	return true
}

func pathID(r *http.Request) int64 {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 { return 0 }
	return id
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") { log.Printf("%s %s", r.Method, r.URL.Path) }
		next.ServeHTTP(w, r)
	})
}
