# MiniDevOpsHub Dashboard UI Plan

## Sections

1. **Apps Section (Control Panel)**
   - List of apps: name, worker, version, status, URL
   - Actions: Deploy, Rollback, Cleanup, View Logs

2. **Linked Repository Section**
   - Repo URL, branch, last commit, commit time

3. **Deployments Section**
   - Version history, blue/green state, active version

4. **Logs Section**
   - Terminal-style, live updates, scrollable

5. **Workers Section**
   - Worker name, IP, status

6. **Help Section**
   - How to use, steps to deploy/manage

## User Flow
- Create app (enter repo URL)
- Select worker
- Deploy
- View logs live
- Access app at /appName
- Rollback, cleanup, etc.

## API
- Use endpoints from ../api/openapi.yaml
- All API calls are relative to `/api/`
