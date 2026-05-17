# Quickstart: Blind Map Survival

Local development guide for backend (Go) and Android client (Kotlin).

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.23+ | https://go.dev/dl/ |
| Docker | 24+ | https://docs.docker.com/get-docker/ |
| Android Studio | Ladybug+ | https://developer.android.com/studio |
| JDK | 17+ | bundled with Android Studio |
| wscat | any | `npm install -g wscat` (optional, for manual WS testing) |

---

## Backend — Run Locally

```bash
cd server/

# Download dependencies
go mod download

# Run with default config (port 8080, 20x20 map, 30s turns)
go run ./cmd/server

# Run with custom config
PORT=9090 MAP_SIZE=15 TURN_SECONDS=10 LOG_LEVEL=debug go run ./cmd/server

# Run tests (with race detector)
go test -race -timeout 60s ./...

# Vet
go vet ./...
```

The server logs JSON to stdout. Health check: `curl http://localhost:8080/healthz` → `{"status":"ok"}`.

### Connect manually with wscat

```bash
# Create a room as Alice
wscat -c "ws://localhost:8080/ws?room=TEST01&name=Alice"

# In wscat session:
{"type":"join","ts":0,"data":{"roomCode":"TEST01","playerName":"Alice","clientVersion":"1.0.0"}}
```

---

## Backend — Docker Build

```bash
cd server/

# Build the image
docker build -t blind-map-survival:local .

# Run the container
docker run -p 8080:8080 \
  -e LOG_LEVEL=debug \
  -e MAX_ROOMS=10 \
  blind-map-survival:local
```

Final image size target: < 20MB (distroless base).

---

## Android Client — Run in Emulator

```bash
cd client/

# Sync Gradle
./gradlew :shared:assemble

# Run on connected device or emulator
./gradlew :androidApp:installDebug

# Run shared module tests
./gradlew :shared:test

# Run all tests
./gradlew test
```

Set the server URL in `androidApp/src/main/res/values/config.xml` or via build config:
```xml
<string name="server_url">ws://10.0.2.2:8080/ws</string>
```
(`10.0.2.2` is the Android emulator's loopback alias for the host machine.)

---

## Environment Variables (Backend)

| Variable | Default | Notes |
|----------|---------|-------|
| `PORT` | `8080` | HTTP listen port |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `MAX_ROOMS` | `100` | Protects against memory exhaustion |
| `TURN_SECONDS` | `30` | Per-turn timer |
| `MAP_SIZE` | `20` | Grid dimension (square) |
| `ALLOWED_ORIGINS` | `*` | CORS/CSWSH whitelist; set to production domain in prod |

---

## Full Local Playtest (2 terminals + emulator)

1. Terminal 1: `cd server && go run ./cmd/server`
2. Terminal 2 (optional manual client): `wscat -c "ws://localhost:8080/ws?room=PLAY01&name=Bob"`
3. Android emulator: launch app, enter room code `PLAY01`, join as Alice
4. Use wscat terminal to send Bob's moves; watch Alice's screen update

---

## Deployment (Render.com)

1. Push `server/` to GitHub/GitLab main branch.
2. Render auto-detects `render.yaml` at repo root and builds the Docker image.
3. First deploy: visit `https://<app>.onrender.com/healthz` — cold start takes ~30s.
4. Set env vars in Render dashboard (or add to `render.yaml` `envVars` section).

CI check before pushing:
```bash
cd server && go vet ./... && go test -race -timeout 60s ./...
```
