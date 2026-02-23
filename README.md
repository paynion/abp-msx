# abp-msx

Lightweight TUI to run and manage [ABP Framework](https://abp.io) microservice solutions from the CLI. Start containers and .NET applications, view logs, and control services from a single terminal.

![Demo](demo.gif)

## Purpose

- **Lightweight** — Single binary, no extra runtime; uses your existing Docker and .NET SDK.
- **ABP-native** — Reads run profiles from `etc/abp-studio/run-profiles/*.abprun.json` (ABP Studio format).
- **CLI-first** — Run from the project root (or with `-d`), pick a profile, then use the TUI to start/stop services and view logs.

## Requirements

- An ABP solution with **ABP Studio** run profiles: directory `etc/abp-studio` and `etc/abp-studio/run-profiles/*.abprun.json`.
- **Docker** (for container services) and **.NET SDK** (for .NET applications), as defined in your profile.

## Install

1. Open the [Releases](https://github.com/paynion/abp-msx/releases) page and download the executable that matches your operating system and architecture (e.g. `abp-msx-linux-amd64`, `abp-msx-darwin-arm64`, `abp-msx-windows-amd64.exe`).
2. Rename the file to **`abp-msx`** (on Windows: **`abp-msx.exe`**).
3. Move it to a directory that is in your [PATH](https://en.wikipedia.org/wiki/PATH_(variable)) so you can run it from any terminal:
   - **Linux / macOS:** e.g. `~/bin` or `/usr/local/bin`  
     `mv abp-msx-linux-amd64 ~/bin/abp-msx` then ensure `~/bin` is in your PATH.
   - **Windows:** e.g. `C:\Program Files\abp-msx` or a folder already in PATH — add that folder to [System PATH](https://docs.microsoft.com/en-us/windows/win32/progthand/environment-variables) if needed.

After that, you can run from any terminal:

```bash
abp-msx -d /path/to/your/abp-solution
```

### Build from source

If you prefer to build yourself (requires [Go 1.21+](https://go.dev/dl/)):

```bash
git clone https://github.com/paynion/abp-msx.git
cd abp-msx
go build -o abp-msx .
# then move abp-msx to a directory in your PATH (see above)
```

Cross-build for all platforms (see `Makefile`):

```bash
make all   # outputs to dist/
```

## Usage

From any directory, point to your ABP solution root:

```bash
abp-msx -d /path/to/your/abp-solution
```

If you are already inside the ABP solution root (directory that contains `etc/abp-studio`), you can run:

```bash
abp-msx
```

- If there is **one** run profile, it is used automatically.
- If there are **several**, you choose one at startup, then the TUI opens.

### TUI shortcuts

| Key | Action |
|-----|--------|
| **↑↓** | Move selection |
| **Enter** | Start / stop selected service (or section) |
| **L** | View logs for selected service |
| **O** | Open service URL in browser |
| **K** | Kill process on service port |
| **F** | Open service folder in file manager |
| **S** | Start all services |
| **X** | Stop all services (and cancel retries) |
| **q** | Quit |

Logs view: **G** or **g** to resume auto-scroll, **q** to go back.

## How it works

1. Finds project root (current dir or `-d`) by looking for `etc/abp-studio`.
2. Discovers run profiles in `etc/abp-studio/run-profiles/*.abprun.json`.
3. Loads the chosen profile and builds the service list (containers + applications from the config).
4. TUI shows groups (e.g. Containers, Applications, Core Services). You start/stop per service, per section, or all.
5. Logs are read from the project’s log directory; container logs come from `docker logs`.