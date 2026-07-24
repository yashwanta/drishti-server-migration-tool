# Troubleshooting

Common issues and how to resolve them.

## The page loads but is blank or flashes

**Symptom:** `http://localhost:5173` opens but shows a blank screen or
flashes repeatedly.

**Cause:** Usually a JavaScript runtime error (for example, a missing data
field) or the backend is not running.

**Fix:**
1. Press `F12` to open browser DevTools, check the **Console** tab for red errors.
2. An **Application Error** screen with a red stack trace means a render crash.
   Copy the error and report it.
3. Verify the backend is up: open `http://localhost:8180/healthz`. You should
   see `{"status":"ok",...}`.
4. Hard refresh: `Ctrl+Shift+R`.

## "Cannot read properties of null (reading 'length')"

**Cause:** The API returned `null` for an array field (a known Go JSON
behavior for nil slices) and the frontend tried to read `.length` on it.

**Fix:** This was fixed in the backend (all slices serialize as `[]`) and the
frontend (defensive `arr()` helper). If you see it again, the backend image
may be stale. Restart the backend container:

```powershell
podman rm hs-backend --force
podman run -d --pod hypershift --name hs-backend -e DRISHTI_MODE=mock -e DRISHTI_HTTP_ADDR=:8080 localhost/hypershift-backend:0.1
```

## No VMs or nodes appear

**Cause:** The backend returned inventory but it is empty, or the connection
inventory failed to load.

**Fix:**
1. Check the connections list: `http://localhost:8180/api/v1/connections`.
2. Check a specific inventory: `http://localhost:8180/api/v1/connections/conn-vmware-lab/inventory`.
3. If inventory is empty, the mock provider may not recognize the connection ID.
   Remove and re-add the connection via the UI.
4. Click the **Refresh** button on the dashboard.

## Port 8080 is already in use

**Symptom:** The backend fails to start or another service occupies port 8080.

**Fix:** The Podman helper publishes the backend on **8180** (mapped to the
container's 8080) to avoid conflicts. If running natively, set
`DRISHTI_HTTP_ADDR=:8180` before `go run`.

## "connection not found" (404)

**Cause:** The connection ID does not exist. This happens after deleting a
connection whose inventory is still cached in the browser.

**Fix:** Click **Refresh**. The dashboard reloads all connections and
inventories from the server.

## Podman commands hang or time out

**Cause:** Podman on Windows (WSL) can be slow on first build or stop.

**Fix:**
- Builds: be patient the first time (Go modules and npm are cached afterward).
- Stops: `podman pod stop` may take 10+ seconds. If it hangs, use
  `podman pod rm hypershift --force`.
- Verify the machine is running: `podman machine list`.

## CORS or API proxy errors

**Symptom:** The browser console shows CORS errors or the UI cannot reach
`/api/v1/...`.

**Cause:** The nginx reverse proxy in the frontend container is not forwarding
`/api` to the backend.

**Fix:**
1. Verify the frontend container has the nginx config:
   `podman exec hs-frontend cat /etc/nginx/conf.d/default.conf`
2. It should contain a `location /api/` block proxying to `127.0.0.1:8080`.
3. Reload nginx: `podman exec hs-frontend nginx -s reload`.
4. When running native dev (`npm run dev`), Vite proxies `/api` automatically.

## Resetting everything

To start completely fresh:

```powershell
.\scripts\podman-run.ps1 down
podman rmi localhost/hypershift-backend:0.1 localhost/hypershift-frontend:0.1 localhost/hypershift-worker:0.1
.\scripts\podman-run.ps1 build
.\scripts\podman-run.ps1 up
```

This removes all containers, images, and any in-memory data, then rebuilds.