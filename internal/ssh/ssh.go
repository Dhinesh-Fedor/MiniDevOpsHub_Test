package ssh

import "fmt"

// SSHService executes commands on remote worker nodes.
type SSHService struct{}

func NewSSHService() *SSHService {
	return &SSHService{}
}

func (s *SSHService) RunCommand(ip, user, keyPath, cmd string) (string, error) {
	// TODO: Implement SSH command execution (use golang.org/x/crypto/ssh or os/exec for demo)
	fmt.Printf("[SSH] Would run on %s: %s\n", ip, cmd)
	return "", nil
}
