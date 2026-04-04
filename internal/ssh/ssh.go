package ssh

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// SSHService executes commands on remote worker nodes.
type SSHService struct{}

func NewSSHService() *SSHService {
	return &SSHService{}
}

// RunCommand executes a command on a remote worker via SSH
// On Windows/dev, it simulates remote execution
// On Linux, it uses actual SSH
func (s *SSHService) RunCommand(ip, user, keyPath, cmd string) (string, error) {
	var command *exec.Cmd

	if runtime.GOOS == "windows" {
		// On Windows dev environment, simulate SSH by running command locally
		// In production, you would use SSH (e.g., with plink or ssh.exe)
		command = exec.Command("cmd", "/C", cmd)
	} else {
		// On Linux, use actual SSH
		sshCmd := fmt.Sprintf("ssh -i %s %s@%s '%s'", keyPath, user, ip, cmd)
		command = exec.Command("sh", "-c", sshCmd)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()

	output := stdout.String() + stderr.String()
	if err != nil {
		return output, fmt.Errorf("ssh command failed on %s: %w", ip, err)
	}
	return output, nil
}

// ExecuteDeploymentScript runs a multi-line deployment script on worker
func (s *SSHService) ExecuteDeploymentScript(ip, user, keyPath, scripts string) (string, error) {
	var command *exec.Cmd

	if runtime.GOOS == "windows" {
		// On Windows, run commands locally for demo
		command = exec.Command("cmd", "/C", scripts)
	} else {
		// On Linux, use SSH with bash
		command = exec.Command("bash", "-c", fmt.Sprintf("ssh -i %s %s@%s 'bash -s' << 'EOF'\n%s\nEOF", keyPath, user, ip, scripts))
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n[stderr]\n" + stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("deployment script failed on %s: %w", ip, err)
	}
	return output, nil
}

// ParseLogsFromOutput extracts log lines from command output
func ParseLogsFromOutput(output string) []string {
	if output == "" {
		return []string{}
	}
	lines := strings.Split(output, "\n")
	var logs []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			logs = append(logs, trimmed)
		}
	}
	return logs
}
