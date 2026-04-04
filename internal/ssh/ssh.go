package ssh

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
)

// SSHService executes commands on remote worker nodes.
type SSHService struct{}

func NewSSHService() *SSHService {
	return &SSHService{}
}

func (s *SSHService) RunCommand(ip, user, keyPath, cmd string) (string, error) {
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.Command("cmd", "/C", cmd)
	} else {
		command = exec.Command("sh", "-lc", cmd)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		return stdout.String() + stderr.String(), fmt.Errorf("ssh command failed on %s: %w", ip, err)
	}
	return stdout.String() + stderr.String(), nil
}
