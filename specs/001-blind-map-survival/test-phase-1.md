# Test Guide: Phase 1 — Setup

**Phase goal**: Project skeleton exists and both build systems can resolve dependencies.
**No game code exists yet** — all tests here are build and structure checks.

---

## Prerequisites

- Go 1.23+ installed (`go version`)
- JDK 17+ installed (`java -version`)
- Docker installed (`docker version`)
- Working directory: repo root (`D:\kotlin\mapTracker`)

---

## T001 — Directory Structure

### How to test

```powershell
# Verify server directories
Test-Path server\cmd\server      # must be True
Test-Path server\internal\config # must be True
Test-Path server\internal\hub    # must be True
Test-Path server\internal\room   # must be True
Test-Path server\internal\game   # must be True
Test-Path server\internal\conn   # must be True
Test-Path server\internal\protocol # must be True

# Verify client directories
Test-Path client\shared\src\commonMain\kotlin\com\blindmap # must be True
Test-Path client\androidApp\src\main # must be True
```

### Expected

All `Test-Path` calls return `True`.

### Wrong if

Any returns `False` — the missing directory must be created before subsequent tasks can target it.

---

## T002 — Go Module Initialization

### How to test

```powershell
cd server
go mod download
go list -m all | Select-String "coder/websocket"
```

### Expected

```
github.com/coder/websocket v1.8.x
```

`go mod download` exits with code 0. `go.sum` file exists in `server/`.

### Wrong if

- `go: module lookup disabled by GOFLAGS`: check network / proxy settings
- `coder/websocket` not listed: `go get github.com/coder/websocket@v1.8` was not run
- `go.sum` missing: re-run `go mod download`

---

## T003–T005 — Gradle Files

### How to test

```powershell
cd ..\client
.\gradlew :shared:assemble 2>&1 | Select-String -Pattern "BUILD"
```

### Expected

```
BUILD SUCCESSFUL in Xs
```

### Wrong if

```
FAILURE: Build failed with an exception.
```

Common causes:
- Missing plugin version in `build.gradle.kts` → add `id("org.jetbrains.kotlin.multiplatform") version "2.0.x"`
- Missing Android SDK → set `ANDROID_HOME` or let Android Studio install it
- Ktor dependency not found → check artifact ID is `ktor-client-websockets` not `ktor-websockets`

---

## T006 — AndroidManifest.xml

### How to test

Open `client/androidApp/src/main/AndroidManifest.xml` and verify:
- `<uses-permission android:name="android.permission.INTERNET"/>` is present
- `MainActivity` is declared as launcher activity

### Wrong if

App builds but crashes immediately on network call (missing INTERNET permission).

---

## T007 — Dockerfile Syntax

### How to test

```powershell
cd ..\server
docker build --no-cache -t blind-map-survival:test . 2>&1 | Select-String -Pattern "error|Error|ERRO" -CaseSensitive
```

No game code exists yet — the build will fail at the `go build` step because source files are empty. That is expected. What we are checking:

```powershell
docker build . 2>&1 | Select-String "FROM"
```

Expected: Dockerfile is parsed without syntax errors (the first FROM line is processed).

### Wrong if

```
failed to parse Dockerfile: ...
```

This means Dockerfile has a syntax error (missing COPY, wrong CMD format, etc.).

---

## T008 — render.yaml Syntax

### How to test

Read `server/render.yaml` and verify these fields are present:
- `services[0].type: web`
- `services[0].region: singapore`
- `services[0].plan: free`
- `services[0].runtime: docker`
- One envVar entry with key `PORT` and value `"8080"`

### Wrong if

Render rejects the file on push. Common issues:
- `runtime: docker` missing → Render tries to auto-detect language
- `region: singapore` misspelled → defaults to US region
- `plan: free` missing → prompts for billing

---

## Phase 1 Pass Criteria

| Check | Command | Expected result |
|---|---|---|
| All directories exist | `Test-Path ...` | All True |
| Go dependencies resolved | `go mod download` | Exit code 0 |
| coder/websocket present | `go list -m all` | Contains `coder/websocket` |
| Kotlin shared assembles | `.\gradlew :shared:assemble` | BUILD SUCCESSFUL |
| Dockerfile parseable | `docker build .` | No syntax errors |
| render.yaml has required fields | Manual read | All fields present |

**Proceed to Phase 2 only when all checks pass.**
