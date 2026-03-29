package worker

import (
	"fmt"
)

// WorkerAgent simulates the agent running on a worker node.
type WorkerAgent struct {
	Name string
	IP   string
}

func (w *WorkerAgent) DeployApp(repoURL, branch, appName, slot string) error {
	fmt.Printf("[Worker %s] Deploying %s:%s from %s (%s)\n", w.Name, appName, slot, repoURL, branch)
	// Simulate Docker build/run
	return nil
}

func (w *WorkerAgent) Cleanup(appName, slot string) error {
	fmt.Printf("[Worker %s] Cleaning up %s:%s\n", w.Name, appName, slot)
	return nil
}
