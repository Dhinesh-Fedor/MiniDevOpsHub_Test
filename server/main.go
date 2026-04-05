package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lrw, r)
		if isNoisyPollRequest(r) && lrw.statusCode < http.StatusBadRequest {
			return
		}
		log.Printf("%s %s status=%d duration=%s", r.Method, r.URL.Path, lrw.statusCode, time.Since(start))
	})
}

func isNoisyPollRequest(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet {
		return false
	}
	path := r.URL.Path
	if path == "/healthz" || path == "/projects" || path == "/workers" || strings.HasPrefix(path, "/logs/") {
		return true
	}
	return false
}

func main() {
	log.Println("MiniDevOpsHub starting...")
	http.DefaultServeMux = http.NewServeMux()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serve dashboard only on root paths; do not hijack arbitrary app routes.
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			http.ServeFile(w, r, "frontend/index.html")
			return
		}

		// If the path looks like an API or known backend route, 404
		if strings.HasPrefix(r.URL.Path, "/app") ||
			strings.HasPrefix(r.URL.Path, "/apps") ||
			strings.HasPrefix(r.URL.Path, "/deploy") ||
			strings.HasPrefix(r.URL.Path, "/redeploy") ||
			strings.HasPrefix(r.URL.Path, "/autodeploy") ||
			strings.HasPrefix(r.URL.Path, "/cleanup") ||
			strings.HasPrefix(r.URL.Path, "/clean") ||
			strings.HasPrefix(r.URL.Path, "/rollback") ||
			strings.HasPrefix(r.URL.Path, "/logs") ||
			strings.HasPrefix(r.URL.Path, "/repo") ||
			strings.HasPrefix(r.URL.Path, "/projects") ||
			strings.HasPrefix(r.URL.Path, "/workers") ||
			strings.HasPrefix(r.URL.Path, "/healthz") ||
			strings.HasPrefix(r.URL.Path, "/webhook") {
			http.NotFound(w, r)
			return
		}

		http.NotFound(w, r)
	})
	http.HandleFunc("/app/create", createAppHandler)
	http.HandleFunc("/apps", listAppsHandler)
	http.HandleFunc("/projects", listAppsHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); w.Write([]byte("ok")) })
	http.HandleFunc("/deploy", deployHandler)
	http.HandleFunc("/redeploy", redeployHandler)
	http.HandleFunc("/redeploy/", redeployHandler)
	http.HandleFunc("/autodeploy/", autoDeployHandler)
	http.HandleFunc("/cleanup", cleanupHandler)
	http.HandleFunc("/clean/", cleanupHandler)
	http.HandleFunc("/rollback", rollbackHandler)
	http.HandleFunc("/rollback/", rollbackHandler)
	http.HandleFunc("/logs/stream", logsStreamHandler)
	http.HandleFunc("/logs/", logsStreamHandler)
	http.HandleFunc("/repo/info", repoInfoHandler)
	http.HandleFunc("/repo/", repoInfoHandler)
	http.HandleFunc("/workers", workersHandler)
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	http.HandleFunc("/webhook", webhookHandler)
	// Serve static files from frontend/
	fs := http.FileServer(http.Dir("frontend"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	log.Fatal(http.ListenAndServe(":8080", requestLogger(http.DefaultServeMux)))
}
