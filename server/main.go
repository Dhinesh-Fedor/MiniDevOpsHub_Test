package main

import (
	"log"
	"net/http"
	"strings"
)

func main() {
	log.Println("MiniDevOpsHub starting...")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If the path looks like an API or known backend route, 404
		if strings.HasPrefix(r.URL.Path, "/app") ||
			strings.HasPrefix(r.URL.Path, "/apps") ||
			strings.HasPrefix(r.URL.Path, "/deploy") ||
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
		http.ServeFile(w, r, "frontend/index.html")
	})
	http.HandleFunc("/app/create", createAppHandler)
	http.HandleFunc("/apps", listAppsHandler)
	http.HandleFunc("/projects", listAppsHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); w.Write([]byte("ok")) })
	http.HandleFunc("/deploy", deployHandler)
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
	log.Fatal(http.ListenAndServe(":8080", nil))
}
