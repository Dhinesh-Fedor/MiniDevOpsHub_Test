package deployer

import (
	"fmt"
	"os"
)

// GenerateNginxConfig generates an nginx.conf with proxy rules for all live projects.
func GenerateNginxConfig(projects map[string]struct{ Port int; ProjectID string }) error {
	f, err := os.Create("nginx/nginx.conf")
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, `worker_processes 1;\nevents { worker_connections 1024; }\nhttp {\n    include       mime.types;\n    default_type  application/octet-stream;\n    sendfile        on;\n    keepalive_timeout  65;`)
	for _, p := range projects {
		fmt.Fprintf(f, "\n    server {\n        listen 80;\n        server_name %s.local;\n        location / { proxy_pass http://localhost:%d; }\n    }\n", p.ProjectID, p.Port)
	}
	fmt.Fprintln(f, "}")
	return nil
}
