package main

var dashboardHTML = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>MiniDevOpsHub Dashboard</title>
  <style>
    body { font-family: sans-serif; margin: 0; background: #f6f8fa; }
    .container { max-width: 1200px; margin: 0 auto; padding: 2rem; }
    h1 { color: #22223b; }
    section { background: #fff; border-radius: 8px; margin-bottom: 2rem; box-shadow: 0 2px 8px #0001; padding: 1.5rem; }
    .section-title { font-size: 1.3rem; color: #4a4e69; margin-bottom: 1rem; }
    .logs { background: #22223b; color: #f2e9e4; font-family: monospace; padding: 1rem; border-radius: 6px; height: 200px; overflow-y: auto; }
  </style>
</head>
<body>
  <div class="container">
    <h1>MiniDevOpsHub Dashboard</h1>
    <section>
      <div class="section-title">Apps</div>
      <!-- Apps table here -->
    </section>
    <section>
      <div class="section-title">Linked Repository</div>
      <!-- Repo info here -->
    </section>
    <section>
      <div class="section-title">Deployments</div>
      <!-- Deployments info here -->
    </section>
    <section>
      <div class="section-title">Logs</div>
      <div class="logs">Live logs will appear here...</div>
    </section>
    <section>
      <div class="section-title">Workers</div>
      <!-- Workers info here -->
    </section>
    <section>
      <div class="section-title">Help</div>
      <ul>
        <li>Connect a Git repository and create an app</li>
        <li>Select a worker and deploy</li>
        <li>View logs, rollback, cleanup as needed</li>
      </ul>
    </section>
  </div>
</body>
</html>`)
