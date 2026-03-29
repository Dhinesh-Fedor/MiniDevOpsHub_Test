package main

import (
	"log"
	"net/http"
)

func main() {
	log.Println("MiniDevOpsHub starting...")
	http.HandleFunc("/", dashboardHandler)
	http.HandleFunc("/app/create", createAppHandler)
	http.HandleFunc("/apps", listAppsHandler)
	http.HandleFunc("/deploy", deployHandler)
	http.HandleFunc("/cleanup", cleanupHandler)
	http.HandleFunc("/rollback", rollbackHandler)
	http.HandleFunc("/logs/stream", logsStreamHandler)
	http.HandleFunc("/repo/info", repoInfoHandler)
	http.HandleFunc("/workers", workersHandler)
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	log.Fatal(http.ListenAndServe(":8080", nil))
}
