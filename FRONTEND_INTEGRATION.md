# Frontend Integration Guide

## Expected API Endpoints (for dashboard)

The frontend dashboard automatically works with these endpoints:

### Worker Selection Dropdown
```javascript
GET /workers
Response: [
  {
    "id": "worker-1",
    "name": "worker-1", 
    "ip": "172.31.37.159",
    "status": "active",
    "active_jobs": 0  // Only workers with active_jobs == 0 returned
  },
  // ... other available workers
]
```

**Key:** GET /workers returns **ONLY available workers** (filtered on backend)
- Busy workers (activeJobs > 0) are NOT included
- Frontend dropdown automatically shows only these

### Deploy Request
```javascript
POST /deploy
Content-Type: application/json

{
  "repo_url": "https://github.com/user/repo",
  "branch": "main",
  "worker_id": "worker-1"  // User selected from dropdown
}

Response:
{
  "project_id": "a1b2c3d4",
  "live_url": "http://control-plane/a1b2c3d4/"
}
```

### Projects List
```javascript
GET /projects
Response: [
  {
    "project_id": "a1b2c3d4",
    "name": "repo-name",
    "repo_url": "https://github.com/user/repo",
    "branch": "main",
    "worker_id": "worker-1",
    "worker_name": "worker-1",
    "worker_ip": "172.31.37.159",
    "status": "running",
    "port": 8001,  // Port on WORKER, not control plane
    "live_url": "http://control-plane/a1b2c3d4/"
  }
]
```

### Logs
```javascript
GET /logs/a1b2c3d4
Response: [
  "[INFO] Starting deployment for project a1b2c3d4",
  "[INFO] Repository: https://github.com/user/repo",
  "[INFO] Branch: main",
  "[INFO] Cloning repository...",
  "[INFO] Building Docker image...",
  "[INFO] Container port: 3000",
  "[INFO] Finding available host port...",
  "[INFO] Using host port: 8001",
  "[INFO] Running Docker container...",
  "[INFO] Deployment successful",
  "[PORT] 8001"
]
```

**Note:** Logs contain deployment script output, ending with `[PORT] 8001`

### Rollback
```javascript
POST /rollback/a1b2c3d4
Response:
{
  "project_id": "a1b2c3d4",
  "live_url": "http://control-plane/a1b2c3d4/"
}
```

**Behavior:** Redeploys same repo/branch on **same worker**

### Clean
```javascript
POST /clean/a1b2c3d4
Response:
{
  "status": "cleaned",
  "project_id": "a1b2c3d4"
}
```

**Behavior:** 
- Stops/removes container on worker
- Removes Docker image
- Deletes workspace on worker
- Project removed from dashboard

### Health Check
```javascript
GET /healthz
Response: "ok"
```

---

## Dashboard UI Flow (No Changes Needed)

The frontend already implements the correct flow:

### 1. Connection Status
```javascript
// Poll /healthz every 5 seconds
setInterval(() => {
  fetch('/healthz')
    .then(r => r.ok ? showConnected() : showDisconnected())
}, 5000);
```

### 2. Worker Dropdown Population
```javascript
// On load and after each deploy/clean
fetch('/workers')
  .then(r => r.json())
  .then(workers => {
    // Populate dropdown
    // Only shows available workers (filtered by backend)
    dropdown.innerHTML = workers.map(w => 
      `<option value="${w.id}">${w.name} (${w.ip})</option>`
    ).join('');
  });
```

### 3. Deploy Form
```javascript
// Form fields needed:
// - repo URL input
// - branch input (optional, defaults to "main")
// - worker dropdown (auto-populated from GET /workers)
// - deploy button

// On submit:
fetch('/deploy', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({
    repo_url: document.getElementById('repo').value,
    branch: document.getElementById('branch').value || 'main',
    worker_id: document.getElementById('worker').value
  })
})
.then(r => r.json())
.then(data => {
  // Show project_id and live_url
  // Start log polling
  pollLogs(data.project_id);
  // Open live URL in new tab
  window.open(data.live_url, '_blank');
});
```

### 4. Live Log Polling
```javascript
// Poll /logs/{projectId} every 2 seconds
let pollInterval;

function pollLogs(projectId) {
  pollInterval = setInterval(() => {
    fetch(`/logs/${projectId}`)
      .then(r => r.json())
      .then(logs => {
        // Update logs display
        document.getElementById('logs-panel').innerHTML = 
          logs.map(log => `<div>${escapeHtml(log)}</div>`).join('');
      });
  }, 2000); // Every 2 seconds
}
```

### 5. Project Table
```javascript
// Columns (already exist):
// - project_id
// - worker (from worker_name)
// - status
// - live URL (href to live_url, target="_blank")
// - actions dropdown:
//   - Logs (show logs panel)
//   - Rollback (POST /rollback/{id})
//   - Clean (POST /clean/{id})

// On Rollback:
fetch(`/rollback/${projectId}`, {method: 'POST'})
  .then(r => r.json())
  .then(data => {
    // Refresh projects list
    refreshProjects();
    // Show success
    showNotification('Rollback successful');
  });

// On Clean:
fetch(`/clean/${projectId}`, {method: 'POST'})
  .then(r => r.json())
  .then(data => {
    // Refresh projects list
    refreshProjects();
    // Show success
    showNotification('Project cleaned');
  });
```

---

## Key Points for Frontend

1. **Worker Dropdown is Auto-Filtered**
   - GET /workers returns ONLY available workers
   - No need for frontend-side filtering
   - Busy workers automatically hidden

2. **Live URL Opens in New Tab**
   - Use `window.open(live_url, '_blank')`
   - URL format: `http://control-plane:port/projectID/`
   - Traffic proxied via NGINX to worker_ip:port

3. **Logs Contain Deployment Script Output**
   - Parse deployment progress from logs
   - Last line contains "[PORT] 8001"
   - Stop polling when deployment completes

4. **Worker Info Displayed**
   - `worker_name`: Display in project table
   - `worker_ip`: Available but not shown to user (for debugging)

5. **Port in Project is NOT Control Plane Port**
   - `port: 8001` is the port on the WORKER
   - Control plane doesn't expose this port
   - User accesses via `/projectID/` which NGINX routes

6. **Status Field**
   - Will be "running" for deployed projects
   - Used for UI display (showing status badges)

---

## No UI Changes Required

The frontend dashboard structure remains the same:
- Connection status indicator ✓
- Deploy panel (repo URL, branch, worker dropdown) ✓
- Project table with actions ✓
- Live logs panel ✓
- Worker pool display ✓

The backend changes ensure:
✓ Worker dropdown auto-filters to available only
✓ Deploy uses selected worker (via SSH)
✓ Nginx routes live URL to worker
✓ Rollback re-deploys on same worker
✓ Clean removes from worker and updates state

