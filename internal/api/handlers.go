import (
	...existing code...
	"github.com/minidevopshub/minidevopshub/internal/deployer"
)
package api

import (
	"net/http"
	"encoding/json"
	"io/ioutil"
	"github.com/gin-gonic/gin"
	"github.com/minidevopshub/minidevopshub/internal/ssh"
	"github.com/minidevopshub/minidevopshub/internal/docker"
)

type DeployRequest struct {
	RepoURL string `json:"repo_url"`
	Branch  string `json:"branch"`
	ProjectID string `json:"project_id"`
}

type Project struct {
	ProjectID string `json:"project_id"`
	RepoURL   string `json:"repo_url"`
	Branch    string `json:"branch"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
	WorkerID  string `json:"worker_id"`
}

type Worker struct {
	WorkerID string `json:"worker_id"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Status   string `json:"status"`
}


// In-memory store for demo
var projects = map[string]*Project{}
var logs = map[string][]string{}
var workers = map[string]*Worker{}

func RegisterRoutes(r *gin.Engine) {
	r.GET("/projects", getProjects)
	r.POST("/deploy", deployProject)
	r.GET("/logs/:project_id", getLogs)
	r.GET("/workers", getWorkers)
	r.POST("/workers", registerWorker)
	r.POST("/cleanup", cleanupProject)
	r.POST("/rollback", rollbackProject)
	r.POST("/webhook/github", githubWebhook)
	r.GET("/repo/:project_id", getRepoInfo)
	r.GET("/commits/:project_id", getCommitHistory)
}
// Simulate GitHub webhook handler
func githubWebhook(c *gin.Context) {
	// In production, validate signature and parse event
	c.JSON(http.StatusOK, gin.H{"status": "Webhook received (demo)"})
}

// Simulate repo info endpoint
func getRepoInfo(c *gin.Context) {
	id := c.Param("project_id")
	if p, ok := projects[id]; ok {
		c.JSON(http.StatusOK, gin.H{
			"repo_url": p.RepoURL,
			"branch": p.Branch,
			"last_commit": "abc123",
			"commit_time": "2026-03-29T12:00:00Z",
		})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
}

// Simulate commit history endpoint
func getCommitHistory(c *gin.Context) {
	id := c.Param("project_id")
	if p, ok := projects[id]; ok {
		c.JSON(http.StatusOK, []gin.H{
			{"hash": "abc123", "msg": "Initial commit", "time": "2026-03-29T12:00:00Z"},
			{"hash": "def456", "msg": "Add feature", "time": "2026-03-28T10:00:00Z"},
		})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
}

func getProjects(c *gin.Context) {
	list := []*Project{}
	for _, p := range projects {
		list = append(list, p)
	}
	c.JSON(http.StatusOK, list)
}

func deployProject(c *gin.Context) {
	var req struct {
		RepoURL  string `json:"repo_url"`
		Branch   string `json:"branch"`
		ProjectID string `json:"project_id"`
		WorkerID string `json:"worker_id"`
	}
	body, _ := ioutil.ReadAll(c.Request.Body)
	_ = json.Unmarshal(body, &req)
	sshSvc := ssh.NewSSHService()
	dockerSvc := docker.NewDockerService()
	if req.ProjectID != "" {
		// Redeploy existing
		if p, ok := projects[req.ProjectID]; ok {
			p.Status = "deploying"
			logs[p.ProjectID] = append(logs[p.ProjectID], "Redeployment started...")
			// Simulate SSH+Docker
			if w, ok := workers[p.WorkerID]; ok {
				sshSvc.RunCommand(w.IP, "ubuntu", "~/.ssh/id_rsa", "echo 'Redeploying app'")
				dockerSvc.BuildAndRunContainer(p.RepoURL, p.Branch, p.ProjectID, "blue")
			}
			p.Status = "running"
			logs[p.ProjectID] = append(logs[p.ProjectID], "Deployment finished!")
			c.JSON(http.StatusOK, p)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	// New deploy
	id := "proj-" + string(len(projects)+1)
	p := &Project{
		ProjectID: id,
		RepoURL: req.RepoURL,
		Branch: req.Branch,
		Port: 3000 + len(projects),
		Status: "deploying",
		WorkerID: req.WorkerID,
	}
	projects[id] = p
	logs[id] = []string{"Deployment started..."}
	// Simulate SSH+Docker
	if w, ok := workers[req.WorkerID]; ok {
		sshSvc.RunCommand(w.IP, "ubuntu", "~/.ssh/id_rsa", "echo 'Deploying app'")
		dockerSvc.BuildAndRunContainer(req.RepoURL, req.Branch, id, "blue")
	}
	p.Status = "running"
	logs[id] = append(logs[id], "Deployment finished!")
	// Update NGINX config for all live projects
	liveProjects := map[string]struct{ Port int; ProjectID string }{}
	for _, proj := range projects {
		if proj.Status == "running" {
			liveProjects[proj.ProjectID] = struct{ Port int; ProjectID string }{Port: proj.Port, ProjectID: proj.ProjectID}
		}
	}
	deployer.GenerateNginxConfig(liveProjects)
	c.JSON(http.StatusOK, p)
}

func getWorkers(c *gin.Context) {
	list := []*Worker{}
	for _, w := range workers {
		list = append(list, w)
	}
	c.JSON(http.StatusOK, list)
}

func registerWorker(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		IP   string `json:"ip"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	id := "worker-" + string(len(workers)+1)
	w := &Worker{
		WorkerID: id,
		Name: req.Name,
		IP: req.IP,
		Status: "active",
	}
	workers[id] = w
	c.JSON(http.StatusOK, w)
}

func cleanupProject(c *gin.Context) {
	var req struct{ ProjectID string `json:"project_id"` }
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if p, ok := projects[req.ProjectID]; ok {
		p.Status = "cleaned"
		logs[p.ProjectID] = append(logs[p.ProjectID], "Cleaned up deployment")
		c.JSON(http.StatusOK, p)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
}

func rollbackProject(c *gin.Context) {
	var req struct{ ProjectID string `json:"project_id"` }
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if p, ok := projects[req.ProjectID]; ok {
		logs[p.ProjectID] = append(logs[p.ProjectID], "Rolled back deployment")
		c.JSON(http.StatusOK, p)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
}

func getLogs(c *gin.Context) {
	id := c.Param("project_id")
	if l, ok := logs[id]; ok {
		c.JSON(http.StatusOK, l)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "No logs found"})
}
