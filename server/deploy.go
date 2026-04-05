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
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	nginxRoutesDir  = "/etc/nginx/minidevopshub-routes"
	defaultPortBase = 9000
)

var workerIPs = map[string]string{
	"worker-1": "172.31.37.159",
	"worker-2": "172.31.36.202",
	"worker-3": "172.31.46.150",
}

var (
	stateStore       = storage.NewFileStorage(resolveStateFilePath())
	stateMu          sync.Mutex
	projectsMu       sync.RWMutex
	logsMu           sync.RWMutex
	autoDeployMu     sync.Mutex
	workerSvc        = service.NewInMemoryWorkerService()
	deploySvc        = service.NewInMemoryDeploymentService()
	logSvc           = service.NewInMemoryLogService()
	sshSvc           = ssh.NewSSHService()
	projectLogs      = map[string][]string{}
	autoDeployStopCh = map[string]chan struct{}{}
)

type dashboardState struct {
	Projects map[string]*App          `json:"projects"`
	Logs     map[string][]string      `json:"logs"`
	Workers  []*internalworker.Worker `json:"workers"`
}

func init() {
	loadDashboardState()
	seedDefaultWorkers()
	syncAllWorkerLoads()
	reconcileProjectRuntime()
	syncAutoDeployWatchers()
	saveDashboardState()
}

func resolveStateFilePath() string {
	if custom := strings.TrimSpace(os.Getenv("MINIDEVOPSHUB_STATE_FILE")); custom != "" {
		_ = os.MkdirAll(filepath.Dir(custom), 0755)
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "minidevopshub-state.json"
	}
	stateDir := filepath.Join(home, ".minidevopshub")
	_ = os.MkdirAll(stateDir, 0755)
	return filepath.Join(stateDir, "state.json")
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
	log.Printf("[%s] %s", projectID, trimmed)
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
	stopAutoDeployLoop(projectID)
	projectsMu.Lock()
	delete(projects, projectID)
	projectsMu.Unlock()
}

func updateProjectAutoDeploy(projectID string, enabled bool) {
	projectsMu.Lock()
	defer projectsMu.Unlock()
	if project, ok := projects[projectID]; ok {
		project.AutoDeploy = enabled
	}
}

func updateProjectLastCommitHash(projectID, hash string) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return
	}
	projectsMu.Lock()
	defer projectsMu.Unlock()
	if project, ok := projects[projectID]; ok {
		project.LastCommitHash = hash
	}
}

func updateProjectPrevCommitHash(projectID, hash string) {
	hash = strings.TrimSpace(hash)
	projectsMu.Lock()
	defer projectsMu.Unlock()
	if project, ok := projects[projectID]; ok {
		project.PrevCommitHash = hash
	}
}

func syncAutoDeployWatchers() {
	for _, project := range listProjectsSnapshot() {
		if project == nil {
			continue
		}
		if project.AutoDeploy {
			startAutoDeployLoop(project.ProjectID)
		}
	}
}

func stopAutoDeployLoop(projectID string) {
	autoDeployMu.Lock()
	defer autoDeployMu.Unlock()
	if stop, ok := autoDeployStopCh[projectID]; ok {
		close(stop)
		delete(autoDeployStopCh, projectID)
	}
}

func startAutoDeployLoop(projectID string) {
	autoDeployMu.Lock()
	if _, exists := autoDeployStopCh[projectID]; exists {
		autoDeployMu.Unlock()
		return
	}
	stop := make(chan struct{})
	autoDeployStopCh[projectID] = stop
	autoDeployMu.Unlock()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				project, ok := getProject(projectID)
				if !ok || project == nil {
					stopAutoDeployLoop(projectID)
					return
				}
				if !project.AutoDeploy || project.Status == "building" {
					continue
				}
				head, err := resolveRemoteHead(project.RepoURL, project.Branch)
				if err != nil {
					appendProjectLogLine(projectID, fmt.Sprintf("[WARN] auto-deploy check failed: %v", err))
					continue
				}
				if project.LastCommitHash == "" {
					updateProjectLastCommitHash(projectID, head)
					saveDashboardState()
					continue
				}
				if head == project.LastCommitHash {
					continue
				}
				appendProjectLogLine(projectID, fmt.Sprintf("[INFO] auto-deploy detected new commit %s", head))
				if _, err := redeployProject(projectID, ""); err != nil {
					appendProjectLogLine(projectID, fmt.Sprintf("[WARN] auto-deploy redeploy failed: %v", err))
					continue
				}
			}
		}
	}()
}

func setAutoDeploy(projectID string, enabled bool) (*App, error) {
	project, ok := getProject(projectID)
	if !ok || project == nil {
		return nil, service.ErrNotFound
	}
	updateProjectAutoDeploy(projectID, enabled)
	if enabled {
		if head, err := resolveRemoteHead(project.RepoURL, project.Branch); err == nil {
			updateProjectLastCommitHash(projectID, head)
		}
		startAutoDeployLoop(projectID)
		appendProjectLogLine(projectID, "[INFO] auto-deploy enabled")
	} else {
		stopAutoDeployLoop(projectID)
		appendProjectLogLine(projectID, "[INFO] auto-deploy disabled")
	}
	saveDashboardState()
	project, _ = getProject(projectID)
	return project, nil
}

func resolveRemoteHead(repoURL, branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	cmd := exec.Command("git", "ls-remote", repoURL, "refs/heads/"+branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git ls-remote failed: %w", err)
	}
	line := strings.TrimSpace(string(output))
	if line == "" {
		return "", fmt.Errorf("no commit found for branch %s", branch)
	}
	parts := strings.Fields(strings.Split(line, "\n")[0])
	if len(parts) == 0 {
		return "", fmt.Errorf("unexpected git ls-remote output")
	}
	return parts[0], nil
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
		if status == "failed" || status == "cleaned" {
			project.LiveURL = ""
		}
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
		if project != nil && project.WorkerIP == workerIP && project.Status != "failed" && project.Status != "cleaned" {
			count++
		}
	}
	return count
}

func projectCountOnWorkerID(workerID string) int {
	projectsMu.RLock()
	defer projectsMu.RUnlock()
	count := 0
	for _, project := range projects {
		if project != nil && project.WorkerID == workerID && project.Status != "failed" && project.Status != "cleaned" {
			count++
		}
	}
	return count
}

func activeProjectOnWorker(workerID, excludeProjectID string) (string, bool) {
	projectsMu.RLock()
	defer projectsMu.RUnlock()
	for id, project := range projects {
		if project == nil {
			continue
		}
		if project.WorkerID != workerID {
			continue
		}
		if excludeProjectID != "" && id == excludeProjectID {
			continue
		}
		if project.Status == "failed" || project.Status == "cleaned" {
			continue
		}
		return id, true
	}
	return "", false
}

func syncWorkerLoad(workerID string) {
	if strings.TrimSpace(workerID) == "" {
		return
	}
	worker, err := workerSvc.GetWorker(workerID)
	if err != nil || worker == nil {
		return
	}
	count := projectCountOnWorkerID(workerID)
	worker.ActiveJobs = count
	if count > 0 {
		worker.Status = "busy"
	} else {
		worker.Status = "active"
	}
	_ = workerSvc.UpdateWorker(worker)
}

func syncAllWorkerLoads() {
	workers, err := workerSvc.ListWorkers()
	if err != nil {
		return
	}
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		syncWorkerLoad(worker.ID)
	}
}

func isProjectContainerRunning(project *App) bool {
	if project == nil {
		return false
	}
	workerIP := strings.TrimSpace(project.WorkerIP)
	if workerIP == "" {
		if ip, ok := workerIPs[project.WorkerID]; ok {
			workerIP = ip
		}
	}
	if workerIP == "" {
		return false
	}
	checkCmd := fmt.Sprintf("docker ps --filter \"name=^/app-%s$\" --filter \"status=running\" --format '{{.Names}}'", project.ProjectID)
	output, err := sshSvc.RunCommand(workerIP, sshUser, sshKeyPath, checkCmd)
	if err != nil {
		appendProjectLogLine(project.ProjectID, fmt.Sprintf("[WARN] runtime health check failed: %v", err))
		return false
	}
	return strings.Contains(output, "app-"+project.ProjectID)
}

func reconcileProjectRuntime() {
	projects := listProjectsSnapshot()
	nginxChanged := false
	for _, project := range projects {
		if project == nil {
			continue
		}
		if project.Status == "cleaned" || project.Status == "failed" {
			continue
		}
		if isProjectContainerRunning(project) {
			continue
		}
		appendProjectLogLine(project.ProjectID, "[WARN] project runtime missing on startup; marking failed and removing route")
		updateProjectStatus(project.ProjectID, "failed")
		updateProjectLiveURL(project.ProjectID, "")
		if err := removeProjectNginxConfig(project.ProjectID); err == nil {
			nginxChanged = true
		}
		syncWorkerLoad(project.WorkerID)
	}
	if nginxChanged {
		_ = reloadNginx()
	}
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

func resolvedLiveURL(albHost, workerIP string, port int, projectID string) string {
	albHost = strings.TrimSpace(albHost)
	if albHost != "" {
		return fmt.Sprintf("http://%s/%s/", albHost, projectID)
	}
	return fmt.Sprintf("http://%s:%d/", workerIP, port)
}

func preferredPublicHost(requestHost string) string {
	envHost := strings.TrimSpace(os.Getenv("ALB_HOST"))
	if envHost != "" {
		return envHost
	}
	return strings.TrimSpace(requestHost)
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

func createOrUpdateProject(repoURL, branch, workerID, projectID string, requestHost string, autoDeploy *bool) (*App, error) {
	return createOrUpdateProjectWithRevision(repoURL, branch, workerID, projectID, requestHost, autoDeploy, "")
}

func createOrUpdateProjectWithRevision(repoURL, branch, workerID, projectID string, requestHost string, autoDeploy *bool, revision string) (*App, error) {
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
	if existingProjectID, busy := activeProjectOnWorker(workerID, projectID); busy {
		return nil, fmt.Errorf("worker %s is busy with active project %s", workerID, existingProjectID)
	}
	worker, err := workerSvc.GetWorker(workerID)
	if err != nil {
		worker = &internalworker.Worker{ID: workerID, Name: workerID, IP: workerIP, Status: "active", ActiveJobs: 0}
		_ = workerSvc.CreateWorker(worker)
	}
	worker.IP = workerIP
	_ = workerSvc.UpdateWorker(worker)

	autoDeployEnabled := false
	lastCommitHash := ""
	prevCommitHash := ""

	// If project exists, remove old artifacts from worker
	if existing, ok := getProject(projectID); ok {
		autoDeployEnabled = existing.AutoDeploy
		lastCommitHash = existing.LastCommitHash
		prevCommitHash = existing.PrevCommitHash
		_ = removeRuntimeArtifactsSSH(existing)
	}
	if autoDeploy != nil {
		autoDeployEnabled = *autoDeploy
	}

	port := chooseProjectPort(worker.IP, projectID)
	imageName := fmt.Sprintf("app-%s", projectID)
	containerName := imageName
	workspacePath := fmt.Sprintf("/tmp/%s", projectID)

	hostPort := port
	publicHost := preferredPublicHost(requestHost)

	project := &App{
		ProjectID:      projectID,
		Name:           projectName,
		RepoURL:        repoURL,
		Branch:         branch,
		AutoDeploy:     autoDeployEnabled,
		LastCommitHash: lastCommitHash,
		PrevCommitHash: prevCommitHash,
		WorkerID:       worker.ID,
		WorkerName:     worker.Name,
		WorkerIP:       worker.IP,
		Status:         "building",
		Port:           hostPort,
		LiveURL:        resolvedLiveURL(publicHost, worker.IP, hostPort, projectID),
		WorkspaceDir:   workspacePath,
		ImageName:      imageName,
		ContainerName:  containerName,
	}

	setProject(project)
	if project.AutoDeploy {
		startAutoDeployLoop(projectID)
	} else {
		stopAutoDeployLoop(projectID)
	}
	syncWorkerLoad(worker.ID)
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
	go runDeploy(projectID, repoURL, branch, workerID, publicHost, hostPort, revision)
	saveDashboardState()
	return project, nil
}

func chooseProjectPort(workerIP, projectID string) int {
	if fixed := strings.TrimSpace(os.Getenv("WORKER_FIXED_PORT")); fixed != "" {
		if parsed, err := strconv.Atoi(fixed); err == nil && parsed > 0 {
			return parsed
		}
	}
	if base := strings.TrimSpace(os.Getenv("WORKER_PORT_BASE")); base != "" {
		if parsed, err := strconv.Atoi(base); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultPortBase
}

func runDeploy(projectID, repoURL, branch, workerID, requestHost string, port int, revision string) {
	workerIP := workerIPs[workerID]
	defer func() {
		syncWorkerLoad(workerID)
		saveDashboardState()
	}()

	appendProjectLogLine(projectID, fmt.Sprintf("[INFO] starting deploy on %s (%s:%d)", workerID, workerIP, port))

	remoteCmd := buildRemoteDeployCommand(projectID, repoURL, branch, port, revision)
	appendProjectLogLine(projectID, fmt.Sprintf("[DEBUG] remote deploy command prepared for %s", workerIP))
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
	appendProjectLogLine(projectID, "[DEBUG] ssh deploy process started")

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
	project.LiveURL = resolvedLiveURL(requestHost, workerIP, port, projectID)
	if err := verifyWorkerUpstreamReachable(projectID, workerIP, port); err != nil {
		markProjectFailed(projectID, err)
		return
	}
	if err := writeProjectNginxConfig(project); err != nil {
		markProjectFailed(projectID, err)
		return
	}
	if err := reloadNginx(); err != nil {
		markProjectFailed(projectID, fmt.Errorf("nginx reload failed: %w", err))
		return
	}

	updateProjectStatus(projectID, "running")
	updateProjectLiveURL(projectID, resolvedLiveURL(requestHost, workerIP, port, projectID))
	targetHead := strings.TrimSpace(revision)
	if targetHead == "" {
		if head, err := resolveRemoteHead(repoURL, branch); err == nil {
			targetHead = head
		}
	}
	if targetHead != "" {
		if current, ok := getProject(projectID); ok {
			prev := strings.TrimSpace(current.LastCommitHash)
			if prev != "" && prev != targetHead {
				updateProjectPrevCommitHash(projectID, prev)
			}
		}
		updateProjectLastCommitHash(projectID, targetHead)
	}
	version := nextDeploymentVersion(projectID)
	_ = deploySvc.CreateDeployment(&deployment.Deployment{
		ID:        randomID(),
		AppID:     projectID,
		Version:   version,
		Slot:      defaultLogSlot,
		Status:    "live",
		CreatedAt: time.Now().Format(time.RFC3339),
	})
	appendProjectLogLine(projectID, "[INFO] deployment completed successfully")
	saveDashboardState()
}

func verifyWorkerUpstreamReachable(projectID, workerIP string, port int) error {
	target := fmt.Sprintf("http://%s:%d/", workerIP, port)
	client := &http.Client{Timeout: 4 * time.Second}
	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		resp, err := client.Get(target)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				appendProjectLogLine(projectID, fmt.Sprintf("[INFO] upstream reachable at %s (status=%d)", target, resp.StatusCode))
				return nil
			}
			lastErr = fmt.Errorf("upstream status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		appendProjectLogLine(projectID, fmt.Sprintf("[WARN] upstream probe %d/10 failed for %s: %v", attempt, target, lastErr))
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("worker upstream unreachable (%s). Check worker security group and container startup. last error: %v", target, lastErr)
}

func buildRemoteDeployCommand(projectID, repoURL, branch string, port int, revision string) string {
	branch = strings.TrimSpace(branch)
	cloneCmd := fmt.Sprintf("git clone %s /tmp/%s", repoURL, projectID)
	if branch != "" {
		cloneCmd = fmt.Sprintf("git clone --branch %s %s /tmp/%s", branch, repoURL, projectID)
	}
	checkoutCmd := ""
	if rev := strings.TrimSpace(revision); rev != "" {
		checkoutCmd = fmt.Sprintf("git checkout %s", rev)
	}

	return fmt.Sprintf(`set -e
docker rm -f app-%[1]s >/dev/null 2>&1 || true
	sudo rm -rf /tmp/%[1]s || true
%[2]s
cd /tmp/%[1]s
%[4]s

verify_running() {
  if ! docker ps --filter "name=^/app-%[1]s$" --filter "status=running" --format '{{.Names}}' | grep -q "app-%[1]s"; then
    echo "[ERROR] container app-%[1]s is not running" >&2
    docker logs app-%[1]s 2>/dev/null | tail -n 120 >&2 || true
    exit 1
  fi
}

run_node_container() {
	APP_DIR="$1"
	echo "[INFO] starting node app from $APP_DIR"
	if [ -f "/tmp/%[1]s/$APP_DIR/vite.config.js" ] || [ -f "/tmp/%[1]s/$APP_DIR/vite.config.ts" ] || grep -q '"vite"' "/tmp/%[1]s/$APP_DIR/package.json" || grep -q '"react-scripts"' "/tmp/%[1]s/$APP_DIR/package.json"; then
		echo "[INFO] frontend framework detected; building production static assets"
		if [ -f "/tmp/%[1]s/$APP_DIR/vite.config.js" ] || [ -f "/tmp/%[1]s/$APP_DIR/vite.config.ts" ] || grep -q '"vite"' "/tmp/%[1]s/$APP_DIR/package.json"; then
			docker run --rm --user $(id -u):$(id -g) -v /tmp/%[1]s/$APP_DIR:/app -w /app node:18 sh -c "npm install && (npm run build -- --base '/%[1]s/' || npm run build)"
		else
			docker run --rm --user $(id -u):$(id -g) -v /tmp/%[1]s/$APP_DIR:/app -w /app node:18 sh -c "npm install && npm run build"
		fi
		if run_static_site_from "$APP_DIR"; then
			return 0
		fi
		echo "[ERROR] build finished but dist/build folder not found for $APP_DIR" >&2
		exit 1
	fi
	docker run -d --name app-%[1]s -p %[3]d:3000 -e HOST=0.0.0.0 -e PORT=3000 -v /tmp/%[1]s/$APP_DIR:/app -w /app node:18 sh -c "npm install && (npm start || node server.js || node app.js)"
	verify_running
}

run_static_site_from() {
	OUT_DIR="$1"
	cat >/tmp/%[1]s/.minidevopshub-nginx.conf <<'EOF'
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
	if [ -d "$OUT_DIR/dist" ]; then
		echo "[INFO] static dist detected at $OUT_DIR/dist"
		docker run -d --name app-%[1]s -p %[3]d:80 -v /tmp/%[1]s/$OUT_DIR/dist:/usr/share/nginx/html:ro -v /tmp/%[1]s/.minidevopshub-nginx.conf:/etc/nginx/conf.d/default.conf:ro nginx:alpine
		verify_running
		return 0
	fi
	if [ -d "$OUT_DIR/build" ]; then
		echo "[INFO] static build detected at $OUT_DIR/build"
		docker run -d --name app-%[1]s -p %[3]d:80 -v /tmp/%[1]s/$OUT_DIR/build:/usr/share/nginx/html:ro -v /tmp/%[1]s/.minidevopshub-nginx.conf:/etc/nginx/conf.d/default.conf:ro nginx:alpine
		verify_running
		return 0
	fi
	if [ -f "$OUT_DIR/index.html" ]; then
		echo "[INFO] static index detected at $OUT_DIR/index.html"
		docker run -d --name app-%[1]s -p %[3]d:80 -v /tmp/%[1]s/$OUT_DIR:/usr/share/nginx/html:ro -v /tmp/%[1]s/.minidevopshub-nginx.conf:/etc/nginx/conf.d/default.conf:ro nginx:alpine
		verify_running
		return 0
	fi
	return 1
}

generate_auto_dockerfile() {
	APP_DIR="$1"
	echo "[INFO] generating Dockerfile.auto for $APP_DIR"
	cat > Dockerfile.auto <<EOF
FROM node:18
WORKDIR /app
COPY $APP_DIR/package*.json ./
RUN npm install
COPY $APP_DIR/ ./
ENV HOST=0.0.0.0
ENV PORT=3000
EXPOSE 3000
CMD ["sh", "-lc", "npm start -- --host 0.0.0.0 --port 3000 || npm run dev -- --host 0.0.0.0 --port 3000 || (npm run build && npm run preview -- --host 0.0.0.0 --port 3000) || node server.js || node app.js"]
EOF
}

echo "Project structure:"
ls -la
[ -d frontend ] && echo "frontend folder exists" && ls -la frontend

DEPLOY_DONE=0

if [ -f Dockerfile ]; then
  echo "[INFO] Dockerfile detected"
	if docker build -t app-%[1]s . && docker run -d --name app-%[1]s -p %[3]d:3000 app-%[1]s; then
		verify_running
		DEPLOY_DONE=1
	else
		echo "[WARN] Dockerfile build/run failed, trying adaptive detection"
		docker rm -f app-%[1]s >/dev/null 2>&1 || true
	fi
fi

if [ "$DEPLOY_DONE" -eq 0 ] && [ -f package.json ]; then
  echo "[INFO] Node app detected (root)"
	run_node_container .
	DEPLOY_DONE=1
fi

if [ "$DEPLOY_DONE" -eq 0 ] && [ -f frontend/package.json ]; then
  echo "[INFO] Node app detected (frontend/)"
	run_node_container frontend
	DEPLOY_DONE=1
fi

if [ "$DEPLOY_DONE" -eq 0 ] && [ -f build.sh ]; then
  echo "[INFO] build.sh detected"
  chmod +x build.sh
  if [ -d frontend ]; then
    echo "[INFO] running build.sh inside frontend"
    cd frontend && bash ../build.sh
		cd /tmp/%[1]s
		if run_static_site_from frontend; then
			DEPLOY_DONE=1
		fi
  else
    bash build.sh
		if run_static_site_from .; then
			DEPLOY_DONE=1
		fi
  fi
fi

if [ "$DEPLOY_DONE" -eq 0 ]; then
	PKG_FILE=$(find . -maxdepth 4 -name package.json -not -path '*/node_modules/*' | head -1)
	if [ -n "$PKG_FILE" ]; then
		APP_DIR=$(dirname "$PKG_FILE" | sed 's|^\./||')
		[ -z "$APP_DIR" ] && APP_DIR=.
		echo "[INFO] adaptive detection found package.json at $APP_DIR"
		generate_auto_dockerfile "$APP_DIR"
		docker build -f Dockerfile.auto -t app-%[1]s .
		docker run -d --name app-%[1]s -p %[3]d:3000 app-%[1]s
		verify_running
		DEPLOY_DONE=1
	fi
fi

if [ "$DEPLOY_DONE" -eq 0 ]; then
  echo "[ERROR] No supported build configuration found" >&2
  exit 1
fi
`, projectID, cloneCmd, port, checkoutCmd)
}

func readPipe(pipe interface{ Read([]byte) (int, error) }, projectID string) {
	scanner := bufio.NewScanner(pipe)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		appendProjectLogLine(projectID, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		appendProjectLogLine(projectID, fmt.Sprintf("[WARN] log stream read error: %v", err))
	}
}

func markProjectFailed(projectID string, err error) {
	appendProjectLogLine(projectID, fmt.Sprintf("[ERROR] %v", err))
	updateProjectStatus(projectID, "failed")
	updateProjectLiveURL(projectID, "")
	if removeErr := removeProjectNginxConfig(projectID); removeErr != nil {
		appendProjectLogLine(projectID, fmt.Sprintf("[WARN] failed to remove nginx route: %v", removeErr))
	} else {
		if reloadErr := reloadNginx(); reloadErr != nil {
			appendProjectLogLine(projectID, fmt.Sprintf("[WARN] nginx reload after failure cleanup failed: %v", reloadErr))
		}
	}
	if project, ok := getProject(projectID); ok {
		syncWorkerLoad(project.WorkerID)
	}
	saveDashboardState()
}

func nextDeploymentVersion(projectID string) int {
	entries, err := deploySvc.ListDeployments(projectID)
	if err != nil || len(entries) == 0 {
		return 1
	}
	return len(entries) + 1
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
		"docker stop app-%[1]s >/dev/null 2>&1 || true; docker rm app-%[1]s >/dev/null 2>&1 || true; docker rmi app-%[1]s >/dev/null 2>&1 || true; sudo rm -rf /tmp/%[1]s >/dev/null 2>&1 || true",
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
	if strings.TrimSpace(requestHost) == "" {
		if parsed, err := url.Parse(strings.TrimSpace(project.LiveURL)); err == nil && parsed.Host != "" {
			requestHost = parsed.Host
		}
	}
	return createOrUpdateProject(project.RepoURL, project.Branch, project.WorkerID, projectID, requestHost, nil)
}

func rollbackProject(projectID string, requestHost string) (*App, error) {
	config, err := deploySvc.GetLastConfig(projectID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, service.ErrNotFound
	}
	project, ok := getProject(projectID)
	if !ok {
		return nil, service.ErrNotFound
	}
	targetRevision := strings.TrimSpace(project.PrevCommitHash)
	if targetRevision == "" {
		return nil, fmt.Errorf("rollback unavailable: no previous revision recorded")
	}
	if project.AutoDeploy {
		_, _ = setAutoDeploy(projectID, false)
		appendProjectLogLine(projectID, "[INFO] auto-deploy disabled for rollback")
	}
	return createOrUpdateProjectWithRevision(config.RepoURL, config.Branch, config.WorkerID, projectID, requestHost, nil, targetRevision)
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
	syncWorkerLoad(project.WorkerID)
	saveDashboardState()
	return nil
}

func writeProjectNginxConfig(project *App) error {
	if project == nil {
		return nil
	}
	if err := os.MkdirAll(nginxRoutesDir, 0755); err != nil {
		cmd := exec.Command("sudo", "mkdir", "-p", nginxRoutesDir)
		if out, sudoErr := cmd.CombinedOutput(); sudoErr != nil {
			return fmt.Errorf("create nginx routes dir failed: %v (%s)", sudoErr, strings.TrimSpace(string(out)))
		}
	}
	conf := fmt.Sprintf(
		"location = /%s {\n    return 301 /%s/;\n}\n\nlocation /%s/ {\n    proxy_pass http://%s:%d;\n\n    proxy_http_version 1.1;\n\n    proxy_set_header Host $host;\n    proxy_set_header X-Real-IP $remote_addr;\n    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n\n    # FIX: correct rewrite logic\n    rewrite ^/%s/(.*)$ /$1 break;\n\n    # FIX: ensure root works\n    rewrite ^/%s/$ / break;\n\n    proxy_intercept_errors on;\n}\n",
		project.ProjectID,
		project.ProjectID,
		project.ProjectID,
		project.WorkerIP,
		project.Port,
		project.ProjectID,
		project.ProjectID,
	)
	confPath := fmt.Sprintf("%s/%s.conf", nginxRoutesDir, project.ProjectID)
	if err := os.WriteFile(confPath, []byte(conf), 0644); err == nil {
		return nil
	}
	cmd := exec.Command("sudo", "tee", confPath)
	cmd.Stdin = strings.NewReader(conf)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		appendProjectLogLine(project.ProjectID, fmt.Sprintf("[INFO] nginx write output: %s", strings.TrimSpace(string(output))))
	}
	if err != nil {
		return fmt.Errorf("write nginx config failed: %w", err)
	}
	return nil
}

func removeProjectNginxConfig(projectID string) error {
	confPath := fmt.Sprintf("%s/%s.conf", nginxRoutesDir, projectID)
	if err := os.Remove(confPath); err == nil || os.IsNotExist(err) {
		return nil
	}
	cmd := exec.Command("sudo", "rm", "-f", confPath)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		appendProjectLogLine(projectID, fmt.Sprintf("[INFO] nginx remove output: %s", strings.TrimSpace(string(output))))
	}
	if err != nil {
		return fmt.Errorf("remove nginx config failed: %w", err)
	}
	return nil
}

func reloadNginx() error {
	testCmd := exec.Command("sudo", "nginx", "-t")
	output, err := testCmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			storeProjectLogs("system", strings.Split(strings.TrimSpace(string(output)), "\n"))
			log.Printf("nginx syntax check output=%s", strings.TrimSpace(string(output)))
		}
		return err
	}

	reloadCmd := exec.Command("sudo", "nginx", "-s", "reload")
	output, err = reloadCmd.CombinedOutput()
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
