package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/minidevopshub/minidevopshub/internal/worker"
)

func createAppHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Repo string `json:"repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Repo == "" {
		http.Error(w, "name and repo are required", http.StatusBadRequest)
		return
	}
	projectID := randomID()
	project := &App{
		ProjectID: projectID,
		Name:      req.Name,
		RepoURL:   req.Repo,
		Status:    "created",
	}
	setProject(project)
	saveDashboardState()
	writeJSON(w, http.StatusOK, project)
}

func listAppsHandler(w http.ResponseWriter, r *http.Request) {
	list := listProjects()
	writeJSON(w, http.StatusOK, list)
}

func deployHandler(w http.ResponseWriter, r *http.Request) {
	var req DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.RepoURL == "" || req.WorkerID == "" {
		http.Error(w, "repo_url and worker_id are required", http.StatusBadRequest)
		return
	}

	// Check if worker exists and is available
	selectedWorker, err := workerSvc.GetWorker(req.WorkerID)
	if err != nil {
		http.Error(w, "worker not found", http.StatusNotFound)
		return
	}
	if !selectedWorker.IsAvailable() {
		http.Error(w, fmt.Sprintf("worker %s is busy", req.WorkerID), http.StatusConflict)
		return
	}

	// Mark worker as busy
	selectedWorker.ActiveJobs++
	_ = workerSvc.UpdateWorker(selectedWorker)

	project, err := createOrUpdateProject(req.RepoURL, req.Branch, req.WorkerID, req.ProjectID, publicHost(r))
	if err != nil {
		selectedWorker.ActiveJobs--
		if selectedWorker.ActiveJobs < 0 {
			selectedWorker.ActiveJobs = 0
		}
		_ = workerSvc.UpdateWorker(selectedWorker)
		if existing, ok := getProject(req.ProjectID); ok {
			existing.Status = "failed"
			saveDashboardState()
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, DeployResponse{
		ProjectID: project.ProjectID,
		Worker:    project.WorkerID,
		Port:      fmt.Sprintf("%d", project.Port),
		Status:    project.Status,
		LiveURL:   project.LiveURL,
	})
}

func cleanupHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromRequest(r)
	if projectID == "" {
		var req CleanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			projectID = req.ProjectID
		}
	}
	if projectID == "" {
		http.Error(w, "project id required", http.StatusBadRequest)
		return
	}

	// Get project to find worker
	project, ok := getProject(projectID)
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	if err := cleanProject(projectID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Decrement worker's active_jobs after cleanup
	if worker, err := workerSvc.GetWorker(project.WorkerID); err == nil && worker.ActiveJobs > 0 {
		worker.ActiveJobs--
		_ = workerSvc.UpdateWorker(worker)
		saveDashboardState()
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cleaned", "project_id": projectID})
}

func cleanProjectHandler(w http.ResponseWriter, r *http.Request) {
	cleanupHandler(w, r)
}

func rollbackHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromRequest(r)
	if projectID == "" {
		var req RollbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			projectID = req.ProjectID
		}
	}
	if projectID == "" {
		http.Error(w, "project id required", http.StatusBadRequest)
		return
	}

	// Get project to find worker
	project, ok := getProject(projectID)
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if project.Status != "running" {
		http.Error(w, "rollback only allowed for running projects", http.StatusConflict)
		return
	}

	// Mark worker as busy for rollback operation
	worker, err := workerSvc.GetWorker(project.WorkerID)
	if err != nil {
		http.Error(w, "worker not found", http.StatusNotFound)
		return
	}

	worker.ActiveJobs++
	_ = workerSvc.UpdateWorker(worker)

	rolledBackProject, err := rollbackProject(projectID, publicHost(r))
	if err != nil {
		worker.ActiveJobs--
		if worker.ActiveJobs < 0 {
			worker.ActiveJobs = 0
		}
		_ = workerSvc.UpdateWorker(worker)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, DeployResponse{
		ProjectID: rolledBackProject.ProjectID,
		Worker:    rolledBackProject.WorkerID,
		Port:      fmt.Sprintf("%d", rolledBackProject.Port),
		Status:    rolledBackProject.Status,
		LiveURL:   rolledBackProject.LiveURL,
	})
}

func rollbackProjectHandler(w http.ResponseWriter, r *http.Request) {
	rollbackHandler(w, r)
}

func logsStreamHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromRequest(r)
	if projectID == "" {
		projectID = r.URL.Query().Get("project_id")
	}
	if projectID == "" {
		projectID = r.URL.Query().Get("app")
	}
	lines, _ := logSvc.GetLogs(projectID, defaultLogSlot)
	if len(lines) == 0 {
		lines = []string{"No logs found"}
	}
	writeJSON(w, http.StatusOK, lines)
}

// Webhook endpoint for auto-deploy (GitHub, etc.)
func webhookHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "Webhook received (demo)"})
}

func repoInfoHandler(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromRequest(r)
	if projectID == "" {
		projectID = r.URL.Query().Get("project_id")
	}
	project, ok := getProject(projectID)
	if !ok {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"repo_url":    project.RepoURL,
		"branch":      project.Branch,
		"last_commit": "live",
		"commit_time": timeNowString(),
	})
}

func workersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workers, err := workerSvc.ListWorkers()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Filter and return only available workers (active_jobs == 0)
		availableWorkers := []*worker.Worker{}
		for _, w := range workers {
			if w.IsAvailable() {
				availableWorkers = append(availableWorkers, w)
			}
		}
		sort.Slice(availableWorkers, func(i, j int) bool {
			return availableWorkers[i].ID < availableWorkers[j].ID
		})
		writeJSON(w, http.StatusOK, availableWorkers)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
			IP   string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.IP == "" {
			http.Error(w, "name and ip are required", http.StatusBadRequest)
			return
		}
		id := fmt.Sprintf("worker-%d", workerCount()+1)
		workerObj := &worker.Worker{ID: id, Name: req.Name, IP: req.IP, Status: "active", ActiveJobs: 0}
		if err := workerSvc.CreateWorker(workerObj); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		saveDashboardState()
		writeJSON(w, http.StatusCreated, workerObj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func listProjects() []*App {
	return listProjectsSnapshot()
}

func projectIDFromRequest(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	if parts[1] == "stream" || parts[1] == "info" {
		return ""
	}
	switch parts[0] {
	case "clean", "rollback", "logs", "repo":
		return parts[1]
	default:
		return ""
	}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func workerCount() int {
	workers, err := workerSvc.ListWorkers()
	if err != nil {
		return 0
	}
	return len(workers)
}

func timeNowString() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}
