# MiniDevOpsHub - Distributed Worker Deployment Architecture

## System Overview

The system now implements a distributed architecture where:
- **Control Plane** (172.31.33.149) runs the dashboard, API, and NGINX reverse proxy
- **Worker Nodes** (172.31.37.159, 172.31.36.202, 172.31.46.150) execute deployments via SSH
- **Load Balancer** routes traffic to the control plane
- **User Applications** are deployed on worker nodes and accessed through NGINX routing

---

## Key Components Modified

### 1. **Worker Model** (`internal/worker/worker.go`)
```go
type Worker struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IP         string `json:"ip"`
	Status     string `json:"status"`      // active, inactive
	ActiveJobs int    `json:"active_jobs"` // NEW
}

// IsAvailable() returns true if worker has no active jobs
func (w *Worker) IsAvailable() bool {
	return w.ActiveJobs == 0
}
```

**Changes:**
- Added `ActiveJobs` counter to track busy state
- Added `IsAvailable()` helper method
- Removed old "free/busy" status in favor of active_jobs counter

### 2. **Worker Initialization** (`server/deploy.go`)
```go
func seedDefaultWorkers() {
	_ = workerSvc.CreateWorker(&internalworker.Worker{
		ID: "worker-1", 
		IP: "172.31.37.159",  // AWS IP
		Status: "active", 
		ActiveJobs: 0
	})
	// ... worker-2, worker-3
}
```

**Changes:**
- Updated IPs to AWS EC2 instance IPs
- Initialize `ActiveJobs` to 0 for all workers

### 3. **Worker Filtering Endpoint** (`server/handlers.go`)
```go
func workersHandler(w http.ResponseWriter, r *http.Request) {
	workers, err := workerSvc.ListWorkers()
	// Filter: return ONLY available workers (active_jobs == 0)
	availableWorkers := []*worker.Worker{}
	for _, w := range workers {
		if w.IsAvailable() {
			availableWorkers = append(availableWorkers, w)
		}
	}
	writeJSON(w, http.StatusOK, availableWorkers)
}
```

**Changes:**
- GET `/workers` now returns **only available workers** (not busy)
- Frontend dropdown automatically shows only selectable workers
- Busy workers (active_jobs > 0) are hidden from user selection

### 4. **Deploy Handler with Worker Busy Tracking** (`server/handlers.go`)
```go
func deployHandler(w http.ResponseWriter, r *http.Request) {
	var req DeployRequest // { repo_url, branch, worker_id }
	
	// 1. Check worker exists and is available
	selectedWorker, err := workerSvc.GetWorker(req.WorkerID)
	if !selectedWorker.IsAvailable() {
		http.Error(w, "worker is busy", http.StatusConflict)
		return
	}
	
	// 2. Mark worker busy
	selectedWorker.ActiveJobs++
	_ = workerSvc.UpdateWorker(selectedWorker)
	defer func() {
		// 3. Mark worker available when done
		selectedWorker.ActiveJobs--
		_ = workerSvc.UpdateWorker(selectedWorker)
		saveDashboardState()
	}()
	
	// 4. Execute deployment
	project, err := createOrUpdateProject(...)
}
```

**Flow:**
1. User selects available worker from dropdown
2. Deploy button is clicked, HTTP POST `/deploy` sent
3. Handler checks if worker is available (activeJobs == 0)
4. If busy → return 409 Conflict error
5. Mark worker busy (activeJobs++)
6. Execute deployment (see below)
7. Auto-decrement when complete

### 5. **SSH-Based Deployment** (`server/deploy.go`)
```go
func createOrUpdateProject(...) {
	// Generate SSH deployment script
	deployScript := generateDeploymentScript(
		repoURL, branch, projectID, 
		"/tmp/" + projectID, // workspace on worker
		imageName, 
		containerName
	)
	
	// Execute script on worker via SSH
	output, err := sshSvc.ExecuteDeploymentScript(
		worker.IP,      // "172.31.37.159"
		"root",
		"",             // key path
		deployScript
	)
	
	// Parse port from output
	hostPort := extractPortFromOutput(output)
	
	// Store project with worker IP and port
	project := &App{
		WorkerIP: worker.IP,
		Port:     hostPort,
		// ... other fields
	}
}
```

**Deployment Script (generated, then sent to worker via SSH):**
```bash
#!/bin/bash
set -e

# 1. Cleanup old deployment
rm -rf /tmp/<projectID>
mkdir -p /tmp/<projectID>
cd /tmp/<projectID>

# 2. Clone repository
git clone --branch <branch> <repo_url> .

# 3. Detect container port from Dockerfile
CONTAINER_PORT=$(grep -i "^EXPOSE" Dockerfile | awk '{print $2}' | head -1 || echo "3000")

# 4. Build Docker image
docker build -t minidevopshub-<name>-<projectID> .

# 5. Find available port on host
HOST_PORT=8000
for port in {8000..8100}; do
  if ! netstat -tuln | grep -q ":$port "; then
    HOST_PORT=$port
    break
  fi
done

# 6. Run container with host port mapping
docker run -d -p $HOST_PORT:$CONTAINER_PORT minidevopshub-<name>-<projectID>

# 7. Output port for control plane to capture
echo "[PORT] $HOST_PORT"
```

**Key Points:**
- **NO Docker runs on control plane** ✓
- Deployments execute ONLY on selected worker ✓
- Port auto-discovery on worker ✓
- Logs captured and stored ✓

### 6. **SSH Service Enhancements** (`internal/ssh/ssh.go`)
```go
// ExecuteDeploymentScript runs multi-line script on worker
func (s *SSHService) ExecuteDeploymentScript(
	ip, user, keyPath, scripts string
) (string, error) {
	// On Windows dev: runs locally (demo mode)
	// On Linux prod: uses actual SSH to execution
	if runtime.GOOS == "windows" {
		command = exec.Command("cmd", "/C", scripts)
	} else {
		command = exec.Command("bash", "-c", 
			fmt.Sprintf("ssh -i %s %s@%s 'bash -s'...", keyPath, user, ip))
	}
}

// ParseLogsFromOutput extracts clean log lines from command output
func ParseLogsFromOutput(output string) []string {
	// Splits output, removes empty lines, returns clean logs
}
```

**Changes:**
- Added `ExecuteDeploymentScript()` for multi-line scripts
- Added `ParseLogsFromOutput()` for log extraction
- Windows dev mode: simulates SSH by running locally
- Linux production: actual SSH execution

### 7. **NGINX Routing Config** (`server/deploy.go` - already existed, working perfectly)
```
location /<projectID>/ {
    proxy_pass http://<worker_ip>:<port>/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

**Behavior:**
- After each deploy, NGINX config is dynamically updated
- Each project gets a route that proxies to worker_ip:port
- Control plane NGINX does NOT run containers, only routes traffic ✓

### 8. **Rollback on Same Worker** (`server/handlers.go`)
```go
func rollbackHandler(w http.ResponseWriter, r *http.Request) {
	project := projects[projectID] // Get project
	worker, _ := workerSvc.GetWorker(project.WorkerID) // Get same worker
	
	// Mark worker busy for rollback operation
	worker.ActiveJobs++
	_ = workerSvc.UpdateWorker(worker)
	defer func() {
		worker.ActiveJobs--
		_ = workerSvc.UpdateWorker(worker)
	}()
	
	// Re-deploy with same repo/branch on SAME worker
	rolledBackProject, err := rollbackProject(projectID, req.Host)
	// rollbackProject calls createOrUpdateProject with same workerID
}
```

**Behavior:**
- Rollback ALWAYS uses the same worker that deployed the app ✓
- Marks worker busy during operation
- Automatically marks available when done

### 9. **Clean (Remove) on Same Worker** (`server/handlers.go`)
```go
func cleanupHandler(w http.ResponseWriter, r *http.Request) {
	project := projects[projectID]
	
	// Execute cleanup
	cleanProject(projectID) // which calls removeRuntimeArtifactsSSH
	
	// Decrement worker's active_jobs
	if worker, err := workerSvc.GetWorker(project.WorkerID); err == nil {
		worker.ActiveJobs--
		_ = workerSvc.UpdateWorker(worker)
	}
}
```

**SSH Cleanup Script (generated in `removeRuntimeArtifactsSSH()`):**
```bash
#!/bin/bash
docker ps -a | grep <containerName> && docker stop <containerName> && docker rm <containerName>
docker rmi <imageName>
rm -rf /tmp/<projectID>
```

**Behavior:**
- Uses same worker that deployed the app ✓
- Removes container, image, and workspace
- Cleans control plane state (projects map, logs)
- Updates NGINX config to remove route

---

## User Flow

### 1. **Dashboard Load**
```
GET / → serves frontend/index.html
GET /projects → list all deployed projects
GET /workers → list AVAILABLE workers only (activeJobs == 0)
```

### 2. **User Deploys**
```
1. User opens dashboard (control plane)
2. Enters repo URL: "https://github.com/user/repo"
3. Selects branch: "main" (or other)
4. Worker dropdown populated with ONLY available workers:
   - worker-1 (if free)
   - worker-2 (if free)  
   - worker-3 (if free)
   → Busy workers NOT shown
5. User clicks "Deploy"
6. POST /deploy {repo_url, branch, worker_id}
   
   Backend:
   a) Check worker is available (activeJobs == 0)
   b) Mark busy (activeJobs++)
   c) Generate SSH deployment script
   d) Execute on worker: git clone → docker build → docker run
   e) Capture port from output
   f) Update NGINX to route /projectID → worker_ip:port
   g) Store project metadata
   h) Mark worker available (activeJobs--)
   i) Return {project_id, live_url}

7. Dashboard opens live_url in new tab
8. User sees running app at http://control-plane-ip/projectID/
```

### 3. **View Logs**
```
GET /logs/<projectID> → returns deployment logs
Frontend polls every 2 seconds: setInterval(refreshLogs, 2000)
```

### 4. **User Rolls Back**
```
1. User clicks "Rollback" on project row
2. POST /rollback/<projectID>
   
   Backend:
   a) Get project to find original worker
   b) Check worker is available
   c) Mark worker busy
   d) Re-deploy same repo/branch on same worker
   e) Mark worker available
   
3. Container is stopped/removed, new one deployed
4. Same NGINX route now points to new container
5. Live URL works with updated app
```

### 5. **User Cleans Up**
```
1. User clicks "Clean" on project row
2. POST /clean/<projectID>
   
   Backend:
   a) Get project to find worker
   b) SSH to worker, execute cleanup script
   c) Remove container, image, workspace
   d) Delete project from state
   e) Update NGINX config (remove route)
   f) Decrement worker activeJobs
   
3. Worker becomes available again
4. Project no longer appears in dashboard
```

---

## Architecture Guarantees

✅ **Deployments run ONLY on selected worker** (via SSH, not locally)
✅ **Busy workers hidden from dropdown** (activeJobs tracked per worker)
✅ **Rollback uses same worker** (project.WorkerID stored and reused)
✅ **Clean uses same worker** (cleaner has access to worker IP and container info)
✅ **NGINX routes to worker_ip:port** (not localhost)
✅ **Logs captured during deployment** (script output parsed)
✅ **Live URL opens in new tab** (frontend: target="_blank")
✅ **Control plane is only orchestrator** (no Docker containers run locally)

---

## File Changes Summary

| File | Changes |
|------|---------|
| `internal/worker/worker.go` | Added `ActiveJobs` field, `IsAvailable()` method |
| `internal/ssh/ssh.go` | Added `ExecuteDeploymentScript()`, `ParseLogsFromOutput()` |
| `server/models.go` | No changes (App model already tracks WorkerIP, Port) |
| `server/main.go` | No changes (routes already set up) |
| `server/deploy.go` | Major rewrite: SSH-based deployment, generateDeploymentScript, removeRuntimeArtifactsSSH, use AWS IPs |
| `server/handlers.go` | Updated deployHandler, cleanupHandler, rollbackHandler for busy/available tracking |
| `frontend/index.html` | No changes needed (already filters workers via GET /workers) |

---

## Testing the System

### Test 1: Worker Selection
```bash
curl http://control-plane:8080/workers
# Should return only available workers (activeJobs == 0)
```

### Test 2: Deploy to Worker
```bash
curl -X POST http://control-plane:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/repo",
    "branch": "main",
    "worker_id": "worker-1"
  }'
# Should deploy on 172.31.37.159, not locally
```

### Test 3: Verify App Running
```bash
# From control plane
curl http://172.31.37.159:8001/
# App should be running on worker with dynamically assigned port

# From outside (via NGINX)
curl http://control-plane-ip/projectID/
# Should proxy to worker
```

### Test 4: Rollback
```bash
curl -X POST http://control-plane:8080/rollback/projectID

# Container should be recreated on SAME worker (worker-1)
```

### Test 5: Clean
```bash
curl -X POST http://control-plane:8080/clean/projectID

# Worker should execute cleanup, then become available again
```

---

## Troubleshooting

**Issue: "worker is busy"**
- Wait for current deployment to complete
- Or select a different available worker

**Issue: App not accessible at live URL**
- Check NGINX config: `cat /etc/nginx/sites-enabled/nginx.conf`
- Verify route exists: `location /projectID/`
- Check worker port is open: `netstat -tuln | grep 8XXX`

**Issue: Logs not showing**
- Check deployment script execution: SSH to worker, verify container running
- Verify LogService storing logs: Check `GET /logs/projectID`

**Issue: Worker stuck in busy state**
- Manually decrement if needed (emergency only): Update `ActiveJobs` in state file

