package main

import (
	"bufio"
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
	"sort"
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
	projectsMu  sync.RWMutex
	logsMu      sync.RWMutex
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
		projectsMu.Lock()
		projects = map[string]*App{}
		projectsMu.Unlock()
		logsMu.Lock()
		projectLogs = map[string][]string{}
		logsMu.Unlock()
		return
	}
	projectsMu.Lock()
	if state.Projects != nil {
		projects = state.Projects
	} else {
		projects = map[string]*App{}
	}
	projectsMu.Unlock()
	logsMu.Lock()
	if state.Logs != nil {
		projectLogs = state.Logs
		for projectID, lines := range state.Logs {
			_ = logSvc.ReplaceLogs(projectID, defaultLogSlot, lines)
		}
	} else {
		projectLogs = map[string][]string{}
	}
	logsMu.Unlock()
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
	projectsMu.RLock()
	logsMu.RLock()
	state := dashboardState{
		Projects: projects,
		Logs:     projectLogs,
		Workers:  workers,
	}
	projectsMu.RUnlock()
	logsMu.RUnlock()
	_ = stateStore.Save(&state)
}

func storeProjectLogs(projectID string, lines []string) {
	if len(lines) == 0 {
		return
	}
	logsMu.Lock()
	projectLogs[projectID] = append(projectLogs[projectID], lines...)
	logsMu.Unlock()
	_ = logSvc.AppendLog(projectID, defaultLogSlot, lines)
}

func appendProjectLogLine(projectID, line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	storeProjectLogs(projectID, []string{trimmed})
}

func getProject(projectID string) (*App, bool) {
	projectsMu.RLock()
	defer projectsMu.RUnlock()
	project, ok := projects[projectID]
	return project, ok
}

func setProject(project *App) {
	if project == nil {
		return
	}
	projectsMu.Lock()
	projects[project.ProjectID] = project
	projectsMu.Unlock()
}

func deleteProject(projectID string) {
	projectsMu.Lock()
	delete(projects, projectID)
	projectsMu.Unlock()
}

func listProjectsSnapshot() []*App {
	projectsMu.RLock()
	defer projectsMu.RUnlock()
	ids := make([]string, 0, len(projects))
	for id := range projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := make([]*App, 0, len(ids))
	for _, id := range ids {
		list = append(list, projects[id])
	}
	return list
}

func updateProjectStatus(projectID, status string) {
	projectsMu.Lock()
	defer projectsMu.Unlock()
	if project, ok := projects[projectID]; ok {
		project.Status = status
	}
}

func updateProjectLiveURL(projectID, liveURL string) {
	projectsMu.Lock()
	defer projectsMu.Unlock()
	if project, ok := projects[projectID]; ok {
		project.LiveURL = liveURL
	}
}

func updateProjectPort(projectID string, port int) {
	projectsMu.Lock()
	defer projectsMu.Unlock()
	if project, ok := projects[projectID]; ok {
		project.Port = port
	}
}

func projectCountOnWorker(workerIP string) int {
	projectsMu.RLock()
	defer projectsMu.RUnlock()
	count := 0
	for _, project := range projects {
		if project != nil && project.WorkerIP == workerIP {
			count++
		}
	}
	return count
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
	if existing, ok := getProject(projectID); ok {
		_ = removeRuntimeArtifactsSSH(existing)
	}

	port := chooseProjectPort(worker.IP, projectID)
	imageName := fmt.Sprintf("app-%s", projectID)
	containerName := imageName
	workspacePath := fmt.Sprintf("/tmp/%s", projectID)

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
		Status:        "building",
		Port:          hostPort,
		LiveURL:       fmt.Sprintf("http://%s/%s/", albHost, projectID),
		WorkspaceDir:  workspacePath,
		ImageName:     imageName,
		ContainerName: containerName,
	}

	setProject(project)
	appendProjectLogLine(projectID, fmt.Sprintf("[INFO] queued deploy for %s on %s", projectID, workerID))
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
	go runDeploy(projectID, repoURL, branch, workerID, albHost, hostPort)
	saveDashboardState()
	return project, nil
}

func chooseProjectPort(workerIP, projectID string) int {
	used := map[int]bool{}
	projectsMu.RLock()
	for _, p := range projects {
		if p != nil && p.WorkerIP == workerIP {
			used[p.Port] = true
		}
	}
	projectsMu.RUnlock()
	randomBytes := make([]byte, 2)
	_, _ = rand.Read(randomBytes)
	candidate := defaultPortBase + (int(randomBytes[0]) * int(randomBytes[1]) % 2000)
	for used[candidate] || candidate == 0 {
		candidate++
	}
	return candidate
}

func runDeploy(projectID, repoURL, branch, workerID, requestHost string, port int) {
	workerIP := workerIPs[workerID]
	defer func() {
		if worker, err := workerSvc.GetWorker(workerID); err == nil {
			worker.ActiveJobs--
			if worker.ActiveJobs < 0 {
				worker.ActiveJobs = 0
			}
			_ = workerSvc.UpdateWorker(worker)
		}
		saveDashboardState()
	}()

	remoteCmd := buildRemoteDeployCommand(projectID, repoURL, branch, port)
	cmd := exec.Command("ssh", "-i", sshKeyPath, sshUser+"@"+workerIP, remoteCmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		markProjectFailed(projectID, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		markProjectFailed(projectID, err)
		return
	}

	if err := cmd.Start(); err != nil {
		markProjectFailed(projectID, err)
		return
	}

	go readPipe(stdout, projectID)
	go readPipe(stderr, projectID)

	if err := cmd.Wait(); err != nil {
		markProjectFailed(projectID, err)
		return
	}

	project, ok := getProject(projectID)
	if !ok {
		markProjectFailed(projectID, fmt.Errorf("project not found after deploy"))
		return
	}
	project.Port = port
	project.LiveURL = fmt.Sprintf("http://%s/%s/", requestHost, projectID)
	if err := writeProjectNginxConfig(project); err != nil {
		markProjectFailed(projectID, err)
		return
	}

	updateProjectStatus(projectID, "running")
	updateProjectLiveURL(projectID, fmt.Sprintf("http://%s/%s/", requestHost, projectID))
	_ = deploySvc.CreateDeployment(&deployment.Deployment{
		ID:        randomID(),
		AppID:     projectID,
		Version:   1,
		Slot:      defaultLogSlot,
		Status:    "live",
		CreatedAt: time.Now().Format(time.RFC3339),
	})
	_ = reloadNginx()
	appendProjectLogLine(projectID, "[INFO] deployment completed successfully")
	saveDashboardState()
}

func buildRemoteDeployCommand(projectID, repoURL, branch string, port int) string {
	branch = strings.TrimSpace(branch)
	cloneCmd := fmt.Sprintf("git clone %s /tmp/%s", repoURL, projectID)
	if branch != "" {
		cloneCmd = fmt.Sprintf("git clone --branch %s %s /tmp/%s", branch, repoURL, projectID)
	}

	return fmt.Sprintf(`set -e
rm -rf /tmp/%[1]s
%[2]s
cd /tmp/%[1]s
if [ -f Dockerfile ]; then
  echo "[INFO] Dockerfile detected"
	if docker build -t app-%[1]s .; then
		docker run -d --name app-%[1]s -p %[3]d:8080 app-%[1]s
	else
		echo "[WARN] Docker build failed, checking fallback modes"
		if [ -f build.sh ]; then
			echo "[INFO] fallback build.sh detected"
			chmod +x build.sh
			./build.sh
		else
			PKG_FILE=$(find . -maxdepth 3 -name package.json | head -1)
			if [ -n "$PKG_FILE" ]; then
				APP_DIR=$(dirname "$PKG_FILE")
				echo "[INFO] fallback package.json detected at $APP_DIR"
				docker run -d --name app-%[1]s -p %[3]d:3000 -v /tmp/%[1]s/$APP_DIR:/app -w /app node:18 sh -c "npm install && npm start"
			else
				echo "No supported build configuration found" >&2
				exit 1
			fi
		fi
	fi
elif [ -f build.sh ]; then
  echo "[INFO] build.sh detected"
  chmod +x build.sh
  ./build.sh
elif [ -f package.json ]; then
  echo "[INFO] package.json detected"
  docker run -d --name app-%[1]s -p %[3]d:3000 -v /tmp/%[1]s:/app -w /app node:18 sh -c "npm install && npm start"
else
	PKG_FILE=$(find . -maxdepth 3 -name package.json | head -1)
	if [ -n "$PKG_FILE" ]; then
		APP_DIR=$(dirname "$PKG_FILE")
		echo "[INFO] package.json detected at $APP_DIR"
		docker run -d --name app-%[1]s -p %[3]d:3000 -v /tmp/%[1]s/$APP_DIR:/app -w /app node:18 sh -c "npm install && npm start"
	else
		echo "No supported build configuration found" >&2
		exit 1
	fi
fi
`, projectID, cloneCmd, port)
}

func readPipe(pipe interface{ Read([]byte) (int, error) }, projectID string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		appendProjectLogLine(projectID, scanner.Text())
	}
}

func markProjectFailed(projectID string, err error) {
	appendProjectLogLine(projectID, fmt.Sprintf("[ERROR] %v", err))
	updateProjectStatus(projectID, "failed")
	saveDashboardState()
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
		"docker stop app-%[1]s >/dev/null 2>&1 || true; docker rm app-%[1]s >/dev/null 2>&1 || true; docker rmi app-%[1]s >/dev/null 2>&1 || true; rm -rf /tmp/%[1]s || true",
		project.ProjectID,
	)
	output, err := sshSvc.RunCommand(worker.IP, sshUser, sshKeyPath, cleanupCmd)
	storeProjectLogs(project.ProjectID, ssh.ParseLogsFromOutput(output))
	log.Printf("clean project=%s worker=%s ip=%s output=%s", project.ProjectID, project.WorkerID, worker.IP, output)
	if err != nil {
		appendProjectLogLine(project.ProjectID, fmt.Sprintf("[WARN] cleanup command returned error: %v", err))
	}
	return nil
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
	project, ok := getProject(projectID)
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
	if _, ok := getProject(projectID); !ok {
		return nil, service.ErrNotFound
	}
	return createOrUpdateProject(config.RepoURL, config.Branch, config.WorkerID, projectID, requestHost)
}

func cleanProject(projectID string) error {
	project, ok := getProject(projectID)
	if !ok {
		return service.ErrNotFound
	}
	if err := removeRuntimeArtifacts(project, true); err != nil {
		appendProjectLogLine(projectID, fmt.Sprintf("[WARN] cleanup artifacts error: %v", err))
	}
	deleteProject(projectID)
	logsMu.Lock()
	delete(projectLogs, projectID)
	logsMu.Unlock()
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
