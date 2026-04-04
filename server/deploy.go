package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minidevopshub/minidevopshub/internal/deployment"
	internaldocker "github.com/minidevopshub/minidevopshub/internal/docker"
	"github.com/minidevopshub/minidevopshub/internal/service"
	"github.com/minidevopshub/minidevopshub/internal/storage"
	internalworker "github.com/minidevopshub/minidevopshub/internal/worker"
)

const defaultLogSlot = "default"

var (
	stateStore  = storage.NewFileStorage("minidevopshub-state.json")
	stateMu     sync.Mutex
	workerSvc   = service.NewInMemoryWorkerService()
	deploySvc   = service.NewInMemoryDeploymentService()
	logSvc      = service.NewInMemoryLogService()
	dockerSvc   = internaldocker.NewDockerService()
	projectLogs = map[string][]string{}
)

type dashboardState struct {
	Projects map[string]*App          `json:"projects"`
	Logs     map[string][]string      `json:"logs"`
	Workers  []*internalworker.Worker `json:"workers"`
}

func init() {
	loadDashboardState()
	seedDefaultWorkers()
	saveDashboardState()
}

func loadDashboardState() {
	state := dashboardState{}
	if err := stateStore.Load(&state); err != nil {
		projects = map[string]*App{}
		projectLogs = map[string][]string{}
		return
	}
	if state.Projects != nil {
		projects = state.Projects
	} else {
		projects = map[string]*App{}
	}
	if state.Logs != nil {
		projectLogs = state.Logs
		for projectID, lines := range state.Logs {
			_ = logSvc.ReplaceLogs(projectID, defaultLogSlot, lines)
		}
	} else {
		projectLogs = map[string][]string{}
	}
	if state.Workers != nil {
		for _, worker := range state.Workers {
			_ = workerSvc.CreateWorker(worker)
		}
	}
}

func seedDefaultWorkers() {
	workers, _ := workerSvc.ListWorkers()
	if len(workers) > 0 {
		return
	}
	_ = workerSvc.CreateWorker(&internalworker.Worker{ID: "worker-1", Name: "worker-1", IP: "127.0.0.1", Status: "active"})
	_ = workerSvc.CreateWorker(&internalworker.Worker{ID: "worker-2", Name: "worker-2", IP: "127.0.0.1", Status: "active"})
	_ = workerSvc.CreateWorker(&internalworker.Worker{ID: "worker-3", Name: "worker-3", IP: "127.0.0.1", Status: "active"})
}

func saveDashboardState() {
	stateMu.Lock()
	defer stateMu.Unlock()
	workers, _ := workerSvc.ListWorkers()
	state := dashboardState{
		Projects: projects,
		Logs:     projectLogs,
		Workers:  workers,
	}
	_ = stateStore.Save(&state)
}

func storeProjectLogs(projectID string, lines []string) {
	if len(lines) == 0 {
		return
	}
	projectLogs[projectID] = append(projectLogs[projectID], lines...)
	_ = logSvc.AppendLog(projectID, defaultLogSlot, lines)
}

func projectNameFromRepoURL(repoURL string) string {
	parsed, err := url.Parse(repoURL)
	if err != nil || parsed.Path == "" {
		fallback := strings.TrimSuffix(path.Base(repoURL), ".git")
		if fallback == "" || fallback == "." || fallback == string(os.PathSeparator) {
			return "app"
		}
		return fallback
	}
	name := strings.TrimSuffix(path.Base(parsed.Path), ".git")
	if name == "" || name == "." || name == string(os.PathSeparator) {
		return "app"
	}
	return name
}

func publicHost(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return "localhost"
	}
	if strings.Contains(host, ":") {
		parts := strings.Split(host, ":")
		return parts[0]
	}
	return host
}

func buildLiveURL(r *http.Request, projectID string) string {
	return fmt.Sprintf("http://%s/%s/", publicHost(r), projectID)
}

func sanitizeName(input string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)
	cleaned := strings.ToLower(re.ReplaceAllString(input, "-"))
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		return "app"
	}
	return cleaned
}

func randomID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func createOrUpdateProject(repoURL, branch, workerID, projectID string, requestHost string) (*App, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	projectName := projectNameFromRepoURL(repoURL)
	if projectID == "" {
		projectID = randomID()
	}
	worker, err := workerSvc.GetWorker(workerID)
	if err != nil {
		return nil, err
	}

	if existing, ok := projects[projectID]; ok {
		_ = removeRuntimeArtifacts(existing, true)
	}

	workspaceDir, err := os.MkdirTemp("", fmt.Sprintf("minidevopshub-%s-", sanitizeName(projectName)))
	if err != nil {
		return nil, err
	}
	hostPort, logs, err := dockerSvc.BuildAndRunContainer(repoURL, branch, projectName, projectID, workspaceDir)
	storeProjectLogs(projectID, logs)
	if err != nil {
		return nil, err
	}
	imageName := fmt.Sprintf("minidevopshub-%s-%s", sanitizeName(projectName), projectID)
	containerName := imageName

	project := &App{
		ProjectID:     projectID,
		Name:          projectName,
		RepoURL:       repoURL,
		Branch:        branch,
		WorkerID:      worker.ID,
		WorkerName:    worker.Name,
		WorkerIP:      worker.IP,
		Status:        "running",
		Port:          hostPort,
		LiveURL:       fmt.Sprintf("http://%s/%s/", requestHost, projectID),
		WorkspaceDir:  workspaceDir,
		ImageName:     imageName,
		ContainerName: containerName,
	}
	projects[projectID] = project
	_ = workerSvc.UpdateWorker(&internalworker.Worker{ID: worker.ID, Name: worker.Name, IP: worker.IP, Status: "busy"})
	_ = deploySvc.RecordLastConfig(projectID, &service.DeployConfig{
		ProjectID:     projectID,
		RepoURL:       repoURL,
		Branch:        branch,
		WorkerID:      worker.ID,
		WorkerIP:      worker.IP,
		ImageName:     project.ImageName,
		ContainerName: project.ContainerName,
		WorkspaceDir:  workspaceDir,
		HostPort:      hostPort,
		ContainerPort: 3000,
	})
	_ = deploySvc.CreateDeployment(&deployment.Deployment{
		ID:        randomID(),
		AppID:     projectID,
		Version:   len(projectLogs[projectID]) + 1,
		Slot:      defaultLogSlot,
		Status:    "live",
		CreatedAt: time.Now().Format(time.RFC3339),
	})
	updateNginxConfig()
	saveDashboardState()
	return project, nil
}

func redeployProject(projectID string, requestHost string) (*App, error) {
	project, ok := projects[projectID]
	if !ok {
		return nil, service.ErrNotFound
	}
	return createOrUpdateProject(project.RepoURL, project.Branch, project.WorkerID, projectID, requestHost)
}

func rollbackProject(projectID string, requestHost string) (*App, error) {
	config, err := deploySvc.GetLastConfig(projectID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, service.ErrNotFound
	}
	if _, ok := projects[projectID]; !ok {
		return nil, service.ErrNotFound
	}
	return createOrUpdateProject(config.RepoURL, config.Branch, config.WorkerID, projectID, requestHost)
}

func removeRuntimeArtifacts(project *App, removeWorkspace bool) error {
	if project == nil {
		return nil
	}
	if project.ContainerName != "" {
		_ = runShellCommand("docker", "stop", project.ContainerName)
		_ = runShellCommand("docker", "rm", "-f", project.ContainerName)
	}
	if project.ImageName != "" {
		_ = runShellCommand("docker", "rmi", "-f", project.ImageName)
	}
	if removeWorkspace && project.WorkspaceDir != "" {
		_ = os.RemoveAll(project.WorkspaceDir)
	}
	if worker, err := workerSvc.GetWorker(project.WorkerID); err == nil {
		_ = workerSvc.UpdateWorker(&internalworker.Worker{ID: worker.ID, Name: worker.Name, IP: worker.IP, Status: "active"})
	}
	return nil
}

func cleanProject(projectID string) error {
	project, ok := projects[projectID]
	if !ok {
		return service.ErrNotFound
	}
	_ = removeRuntimeArtifacts(project, true)
	delete(projects, projectID)
	delete(projectLogs, projectID)
	_ = logSvc.ClearLogs(projectID)
	_ = deploySvc.DeleteLastConfig(projectID)
	updateNginxConfig()
	saveDashboardState()
	return nil
}

func updateNginxConfig() {
	lines := []string{
		"# Minimal NGINX config for MiniDevOpsHub",
		"worker_processes 1;",
		"events { worker_connections 1024; }",
		"http {",
		"    include       mime.types;",
		"    default_type  application/octet-stream;",
		"    sendfile        on;",
		"    keepalive_timeout  65;",
		"",
		"    server {",
		"        listen       80;",
		"        server_name  localhost;",
		"",
		"        location /api/ {",
		"            proxy_pass http://localhost:8080;",
		"            proxy_set_header Host $host;",
		"            proxy_set_header X-Real-IP $remote_addr;",
		"        }",
		"",
		"        location / {",
		"            root   frontend;",
		"            index  index.html index.htm;",
		"        }",
	}

	projectIDs := make([]string, 0, len(projects))
	for projectID := range projects {
		projectIDs = append(projectIDs, projectID)
	}
	sort.Strings(projectIDs)
	for _, projectID := range projectIDs {
		project := projects[projectID]
		lines = append(lines,
			"",
			fmt.Sprintf("        location /%s/ {", projectID),
			fmt.Sprintf("            proxy_pass http://%s:%d/;", project.WorkerIP, project.Port),
			"            proxy_set_header Host $host;",
			"            proxy_set_header X-Real-IP $remote_addr;",
			"        }",
		)
	}

	lines = append(lines,
		"    }",
		"}",
		"",
	)
	_ = os.WriteFile("nginx.conf", []byte(strings.Join(lines, "\n")), 0644)
	_ = runShellCommand("nginx", "-s", "reload")
}

func runShellCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		storeProjectLogs("system", strings.Split(strings.TrimSpace(string(output)), "\n"))
	}
	return err
}
