package docker

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DockerService builds and runs containers on worker nodes.
type DockerService struct{}

func NewDockerService() *DockerService {
	return &DockerService{}
}

func (d *DockerService) BuildAndRunContainer(repoURL, branch, appName, slot, workspaceDir string) (int, []string, error) {
	logs := []string{}
	if workspaceDir == "" {
		var err error
		workspaceDir, err = os.MkdirTemp("", sanitizeName(appName)+"-workspace-")
		if err != nil {
			return 0, logs, err
		}
	}

	logs = append(logs, fmt.Sprintf("Cloning repo %s (%s)", repoURL, branch))
	cloneArgs := []string{"clone"}
	if branch != "" {
		cloneArgs = append(cloneArgs, "--branch", branch, "--single-branch")
	}
	cloneArgs = append(cloneArgs, repoURL, workspaceDir)
	cloneOutput, err := runCommand("git", cloneArgs...)
	logs = append(logs, cloneOutput...)
	if err != nil {
		return 0, logs, err
	}

	containerPort := detectContainerPort(workspaceDir)
	hostPort, err := findFreePort()
	if err != nil {
		return 0, logs, err
	}

	imageName := fmt.Sprintf("minidevopshub-%s-%s", sanitizeName(appName), slot)
	logs = append(logs, fmt.Sprintf("Building Docker image %s", imageName))
	buildOutput, err := runCommand("docker", "build", "-t", imageName, workspaceDir)
	logs = append(logs, buildOutput...)
	if err != nil {
		return hostPort, logs, err
	}

	containerName := fmt.Sprintf("minidevopshub-%s-%s", sanitizeName(appName), slot)
	runArgs := []string{"run", "-d", "--rm", "--name", containerName, "-p", fmt.Sprintf("%d:%d", hostPort, containerPort), imageName}
	logs = append(logs, fmt.Sprintf("Running container %s on port %d", containerName, hostPort))
	runOutput, err := runCommand("docker", runArgs...)
	logs = append(logs, runOutput...)
	if err != nil {
		return hostPort, logs, err
	}
	logs = append(logs, fmt.Sprintf("Container started: %s", strings.TrimSpace(strings.Join(runOutput, " "))))
	return hostPort, logs, nil
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

func detectContainerPort(workspaceDir string) int {
	dockerfilePath := filepath.Join(workspaceDir, "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return 3000
	}
	re := regexp.MustCompile(`(?im)^\s*EXPOSE\s+(\d+)`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) != 2 {
		return 3000
	}
	port, err := strconv.Atoi(matches[1])
	if err != nil || port <= 0 {
		return 3000
	}
	return port
}

func findFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	address := listener.Addr().(*net.TCPAddr)
	return address.Port, nil
}

func runCommand(command string, args ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	lines := splitLines(string(output))
	if err != nil {
		return lines, fmt.Errorf("%s %v failed: %w", command, args, err)
	}
	return lines, nil
}

func splitLines(output string) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	lines := []string{}
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func init() {
	if runtime.GOOS == "windows" {
		_ = os.Setenv("DOCKER_BUILDKIT", "1")
	}
}
