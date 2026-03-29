package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
)

func createAppHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Repo string `json:"repo"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" || req.Repo == "" {
		w.WriteHeader(400)
		return
	}
	apps[req.Name] = &App{
		Name:        req.Name,
		Repo:        req.Repo,
		Status:      "Created",
		Version:     0,
		ActiveColor: "blue",
	}
	w.WriteHeader(200)
}

func listAppsHandler(w http.ResponseWriter, r *http.Request) {
	var out []*App
	for _, a := range apps {
		out = append(out, a)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func deployHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App    string `json:"app"`
		Worker string `json:"worker"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	app, ok := apps[req.App]
	if !ok {
		w.WriteHeader(404)
		return
	}
	workerIP, ok := workers[req.Worker]
	if !ok {
		w.WriteHeader(404)
		return
	}
	color := "green"
	if app.ActiveColor == "green" {
		color = "blue"
	}
	// Simulate SSH and Docker commands (replace with real SSH in production)
	app.Worker = req.Worker
	app.WorkerIP = workerIP
	app.Version++
	app.Status = "Live"
	app.ActiveColor = color
	logs[app.Name] = append(logs[app.Name], fmt.Sprintf("Deployed version %d to %s", app.Version, color))
	w.WriteHeader(200)
}

func cleanupHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App string `json:"app"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	app, ok := apps[req.App]
	if !ok {
		w.WriteHeader(404)
		return
	}
	app.Status = "Created"
	logs[app.Name] = append(logs[app.Name], "Cleaned up deployment")
	w.WriteHeader(200)
}

func rollbackHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App string `json:"app"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	app, ok := apps[req.App]
	if !ok {
		w.WriteHeader(404)
		return
	}
	if app.ActiveColor == "blue" {
		app.ActiveColor = "green"
	} else {
		app.ActiveColor = "blue"
	}
	logs[app.Name] = append(logs[app.Name], "Rolled back deployment")
	w.WriteHeader(200)
}

func logsStreamHandler(w http.ResponseWriter, r *http.Request) {
	appName := r.URL.Query().Get("app")
	w.Header().Set("Content-Type", "application/json")
	if l, ok := logs[appName]; ok {
		json.NewEncoder(w).Encode(l)
	} else {
		json.NewEncoder(w).Encode([]string{"No logs found"})
	}
	// Optionally: append a new log line for demo
	// logs[appName] = append(logs[appName], fmt.Sprintf("Live log line %d", time.Now().Unix()))
}

// Webhook endpoint for auto-deploy (GitHub, etc.)
func webhookHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Validate signature, parse event, trigger deploy if push event
	w.WriteHeader(200)
	w.Write([]byte("Webhook received (demo)"))
}

func repoInfoHandler(w http.ResponseWriter, r *http.Request) {
	appName := r.URL.Query().Get("app")
	app, ok := apps[appName]
	if !ok {
		w.WriteHeader(404)
		return
	}
	// Simulate git info (replace with real git in production)
	branch := "main"
	commit := "Initial commit"
	if app.Repo != "" {
		cmd := exec.Command("git", "ls-remote", app.Repo, "HEAD")
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			commit = string(bytes.Fields(out)[0])
		}
	}
	json.NewEncoder(w).Encode(map[string]string{
		"repo":   app.Repo,
		"branch": branch,
		"commit": commit,
	})
}

func workersHandler(w http.ResponseWriter, r *http.Request) {
	var out []map[string]string
	for k, v := range workers {
		out = append(out, map[string]string{"name": k, "ip": v})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
