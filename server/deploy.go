package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/minidevopshub/minidevopshub/internal/deployment"
	"github.com/minidevopshub/minidevopshub/internal/service"
	"github.com/minidevopshub/minidevopshub/internal/ssh"
	"github.com/minidevopshub/minidevopshub/internal/storage"
	internalworker "github.com/minidevopshub/minidevopshub/internal/worker"
)

const defaultLogSlot = "default"

const (
	sshUser         = "ubuntu"
	sshKeyPath      = "/home/ubuntu/MiniDevOpsHub-Key.pem"
	nginxSitesDir   = "/etc/nginx/sites-enabled"
	defaultPortBase = 18080
)

var workerIPs = map[string]string{
	"worker-1": "172.31.37.159",
	"worker-2": "172.31.36.202",
	"worker-3": "172.31.46.150",
}

var (
	stateStore  = storage.NewFileStorage("minidevopshub-state.json")
	stateMu     sync.Mutex
	workerSvc   = service.NewInMemoryWorkerService()
	deploySvc   = service.NewInMemoryDeploymentService()
	logSvc      = service.NewInMemoryLogService()
	sshSvc      = ssh.NewSSHService()
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
	_ = workerSvc.CreateWorker(&internalworker.Worker{ID: "worker-1", Name: "worker-1", IP: "172.31.37.159", Status: "active", ActiveJobs: 0})
	_ = workerSvc.CreateWorker(&internalworker.Worker{ID: "worker-2", Name: "worker-2", IP: "172.31.36.202", Status: "active", ActiveJobs: 0})
	_ = workerSvc.CreateWorker(&internalworker.Worker{ID: "worker-3", Name: "worker-3", IP: "172.31.46.150", Status: "active", ActiveJobs: 0})
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

	workerIP, ok := workerIPs[workerID]
	if !ok {
		return nil, fmt.Errorf("unknown worker_id: %s", workerID)
	}
	worker, err := workerSvc.GetWorker(workerID)
	if err != nil {
		worker = &internalworker.Worker{ID: workerID, Name: workerID, IP: workerIP, Status: "active", ActiveJobs: 0}
		_ = workerSvc.CreateWorker(worker)
	}
	worker.IP = workerIP
	_ = workerSvc.UpdateWorker(worker)

	// If project exists, remove old artifacts from worker
	if existing, ok := projects[projectID]; ok {
		_ = removeRuntimeArtifactsSSH(existing)
	}

	port := chooseProjectPort(worker.IP, projectID)
	imageName := fmt.Sprintf("app-%s", projectID)
	containerName := imageName
	workspacePath := fmt.Sprintf("/tmp/%s", projectID)
	cmd := generateDeploySSHCommand(repoURL, branch, projectID, port)

	output, err := sshSvc.RunCommand(worker.IP, sshUser, sshKeyPath, cmd)
	logs := ssh.ParseLogsFromOutput(output)
	storeProjectLogs(projectID, logs)
	log.Printf("deploy project=%s worker=%s ip=%s output=%s", projectID, workerID, worker.IP, output)

	if err != nil {
		return nil, fmt.Errorf("deployment on worker %s failed: %w", workerID, err)
	}

	hostPort := port
	albHost := strings.TrimSpace(os.Getenv("ALB_HOST"))
	if albHost == "" {
		albHost = requestHost
	}

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
		LiveURL:       fmt.Sprintf("http://%s/%s/", albHost, projectID),
		WorkspaceDir:  workspacePath,
		ImageName:     imageName,
		ContainerName: containerName,
	}

	projects[projectID] = project
	_ = deploySvc.RecordLastConfig(projectID, &service.DeployConfig{
		ProjectID:     projectID,
		RepoURL:       repoURL,
		Branch:        branch,
		WorkerID:      worker.ID,
		WorkerIP:      worker.IP,
		ImageName:     project.ImageName,
		ContainerName: project.ContainerName,
		WorkspaceDir:  workspacePath,
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
	if err := writeProjectNginxConfig(project); err != nil {
		return nil, err
	}
	if err := reloadNginx(); err != nil {
		return nil, err
	}
	saveDashboardState()
	return project, nil
}

func generateDeploySSHCommand(repoURL, branch, projectID string, port int) string {
	if strings.TrimSpace(branch) == "" {
		return fmt.Sprintf(
			"rm -rf /tmp/%[1]s && git clone %[2]s /tmp/%[1]s && cd /tmp/%[1]s && docker build -t app-%[1]s . && docker run -d --name app-%[1]s -p %[3]d:8080 app-%[1]s",
			projectID,
			repoURL,
			port,
		)
	}
	return fmt.Sprintf(
		"rm -rf /tmp/%[1]s && git clone --branch %[2]s %[3]s /tmp/%[1]s && cd /tmp/%[1]s && docker build -t app-%[1]s . && docker run -d --name app-%[1]s -p %[4]d:8080 app-%[1]s",
		projectID,
		branch,
		repoURL,
		port,
	)
}

func chooseProjectPort(workerIP, projectID string) int {
	used := map[int]bool{}
	for _, p := range projects {
		if p != nil && p.WorkerIP == workerIP {
			used[p.Port] = true
		}
	}
	randomBytes := make([]byte, 2)
	_, _ = rand.Read(randomBytes)
	candidate := defaultPortBase + (int(randomBytes[0]) * int(randomBytes[1]) % 2000)
	for used[candidate] || candidate == 0 {
		candidate++
	}
	return candidate
}

func removeRuntimeArtifactsSSH(project *App) error {
	if project == nil {
		return nil
	}

	worker, err := workerSvc.GetWorker(project.WorkerID)
	if err != nil {
		return err
	}

	cleanupCmd := fmt.Sprintf(
		"docker stop app-%[1]s && docker rm app-%[1]s && docker rmi app-%[1]s && rm -rf /tmp/%[1]s",
		project.ProjectID,
	)
	output, err := sshSvc.RunCommand(worker.IP, sshUser, sshKeyPath, cleanupCmd)
	storeProjectLogs(project.ProjectID, ssh.ParseLogsFromOutput(output))
	log.Printf("clean project=%s worker=%s ip=%s output=%s", project.ProjectID, project.WorkerID, worker.IP, output)
	return err
}

func removeRuntimeArtifacts(project *App, removeWorkspace bool) error {
	if project == nil {
		return nil
	}
	if err := removeRuntimeArtifactsSSH(project); err != nil {
		return err
	}
	if removeWorkspace && project.WorkspaceDir != "" {
		_ = os.RemoveAll(project.WorkspaceDir)
	}
	return nil
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

func cleanProject(projectID string) error {
	project, ok := projects[projectID]
	if !ok {
		return service.ErrNotFound
	}
	if err := removeRuntimeArtifacts(project, true); err != nil {
		return err
	}
	delete(projects, projectID)
	delete(projectLogs, projectID)
	_ = logSvc.ClearLogs(projectID)
	_ = deploySvc.DeleteLastConfig(projectID)
	_ = removeProjectNginxConfig(projectID)
	_ = reloadNginx()
	saveDashboardState()
	return nil
}

func writeProjectNginxConfig(project *App) error {
	if project == nil {
		return nil
	}
	conf := fmt.Sprintf(
		"location /%s/ {\n    proxy_pass http://%s:%d/;\n    proxy_set_header Host $host;\n    proxy_set_header X-Real-IP $remote_addr;\n}\n",
		project.ProjectID,
		project.WorkerIP,
		project.Port,
	)
	confPath := fmt.Sprintf("%s/%s.conf", nginxSitesDir, project.ProjectID)
	return os.WriteFile(confPath, []byte(conf), 0644)
}

func removeProjectNginxConfig(projectID string) error {
	confPath := fmt.Sprintf("%s/%s.conf", nginxSitesDir, projectID)
	if err := os.Remove(confPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		storeProjectLogs("system", strings.Split(strings.TrimSpace(string(output)), "\n"))
		log.Printf("nginx reload output=%s", strings.TrimSpace(string(output)))
	}
	return err
}

func runShellCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		storeProjectLogs("system", strings.Split(strings.TrimSpace(string(output)), "\n"))
	}
	return err
}
