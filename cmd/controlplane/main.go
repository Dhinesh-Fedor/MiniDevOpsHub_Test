package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/minidevopshub/minidevopshub/internal/deployment"
	"github.com/minidevopshub/minidevopshub/internal/service"
)

var (
	appSvc    *service.InMemoryAppService
	workerSvc *service.InMemoryWorkerService
	deploySvc *service.InMemoryDeploymentService
	logSvc    *service.InMemoryLogService
)

func main() {
	log.Println("MiniDevOpsHub Control Plane starting...")
	rand.Seed(time.Now().UnixNano())

	appSvc = service.NewInMemoryAppService()
	workerSvc = service.NewInMemoryWorkerService()
	deploySvc = service.NewInMemoryDeploymentService()
	logSvc = service.NewInMemoryLogService()

	r := chi.NewRouter()

	r.Get("/healthz", healthHandler)

	// App endpoints
	r.Post("/api/apps", createAppHandler)
	r.Get("/api/apps", listAppsHandler)
	r.Get("/api/apps/{id}", getAppHandler)
	r.Delete("/api/apps/{id}", deleteAppHandler)

	// Worker endpoints
	r.Get("/api/workers", listWorkersHandler)
	r.Post("/api/workers", createWorkerHandler)
	r.Get("/api/workers/{id}", getWorkerHandler)

	// Deployment endpoints
	r.Post("/api/apps/{id}/deploy", deployAppHandler)
	r.Post("/api/apps/{id}/rollback", rollbackAppHandler)
	r.Get("/api/apps/{id}/deployments", listDeploymentsHandler)

	// Log endpoints
	r.Get("/api/apps/{id}/logs", getLogsHandler)
	r.Get("/api/apps/{id}/build-logs", getBuildLogsHandler)

	// Repo info
	r.Get("/api/apps/{id}/repo", getRepoInfoHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// --- App Handlers ---
func listAppsHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement listing apps
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("listAppsHandler not implemented"))
}

func getAppHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement get app
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("getAppHandler not implemented"))
}

func deleteAppHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement delete app
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("deleteAppHandler not implemented"))
}

func createWorkerHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement create worker
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("createWorkerHandler not implemented"))
}
func createAppHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		RepoURL string `json:"repo_url"`
		Branch  string `json:"branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	// ...existing logic for creating an app...
}

func listWorkersHandler(w http.ResponseWriter, r *http.Request) {
	workers, _ := workerSvc.ListWorkers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workers)
}

func getWorkerHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	workerObj, err := workerSvc.GetWorker(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workerObj)
}

// --- Utility ---
func randomID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// --- Deployment Handlers ---
func deployAppHandler(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	appObj, err := appSvc.GetApp(appID)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	// Blue-green: deploy to inactive slot
	slot := "green"
	if appObj.ActiveSlot == "green" {
		slot = "blue"
	}
	version := rand.Intn(100000)
	dep := &deployment.Deployment{
		ID:        randomID(),
		AppID:     appID,
		Version:   version,
		Slot:      slot,
		Status:    "live",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	_ = deploySvc.CreateDeployment(dep)
	// Switch active slot
	appObj.ActiveSlot = slot
	appObj.Status = "Live"
	// Simulate log
	logSvc.AppendLog(appID, slot, []string{"Build started...", "Build complete.", "App running."})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dep)
}

func rollbackAppHandler(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	err := deploySvc.Rollback(appID)
	if err != nil {
		http.Error(w, "rollback failed", http.StatusInternalServerError)
		return
	}
	// Also update app active slot
	appObj, _ := appSvc.GetApp(appID)
	if appObj.ActiveSlot == "blue" {
		appObj.ActiveSlot = "green"
	} else {
		appObj.ActiveSlot = "blue"
	}
	appObj.Status = "Live"
	w.WriteHeader(http.StatusNoContent)
}

func listDeploymentsHandler(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	deps, _ := deploySvc.ListDeployments(appID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deps)
}

// --- Log Handlers ---
func getLogsHandler(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	slot := r.URL.Query().Get("slot")
	if slot == "" {
		slot = "blue"
	}
	lines, _ := logSvc.GetLogs(appID, slot)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"lines": lines})
}

func getBuildLogsHandler(w http.ResponseWriter, r *http.Request) {
	// For demo, same as getLogsHandler
	getLogsHandler(w, r)
}

// --- Repo Info Handler ---
func getRepoInfoHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
