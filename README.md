# kurto

[![CI](https://github.com/SomeoneUnlicensed/kurto/actions/workflows/ci.yml/badge.svg)](https://github.com/SomeoneUnlicensed/kurto/actions/workflows/ci.yml)
[![Release](https://github.com/SomeoneUnlicensed/kurto/actions/workflows/release.yml/badge.svg)](https://github.com/SomeoneUnlicensed/kurto/actions/workflows/release.yml)

Minimal container runtime. Cross-platform. No daemon. No dependencies.

Run isolated processes on Linux (namespaces + cgroups v2), Windows (Job Objects), and macOS.

```
kurto r alpine
kurto ps -a
kurto pu nginx:latest
kurto a -f pod.yaml
kurto g pods
```

## Install

### From source

```shell
go install github.com/SomeoneUnlicensed/kurto@latest
```

### Windows

#### PowerShell one-liner

```powershell
iwr -useb https://raw.githubusercontent.com/SomeoneUnlicensed/kurto/main/install/install.ps1 | iex
```

#### Windows installer (NSIS)

Download `kurto-setup-*.exe` from the [Releases page](https://github.com/SomeoneUnlicensed/kurto/releases) and run it.

#### Chocolatey

```shell
choco install kurto
```

#### Manual

```powershell
# Download
Invoke-WebRequest -Uri "https://github.com/SomeoneUnlicensed/kurto/releases/latest/download/kurto-windows-amd64.zip" -OutFile kurto.zip
Expand-Archive kurto.zip -DestinationPath .
mv kurto-windows-amd64.exe kurto.exe

# Add to PATH (Admin)
move kurto.exe C:\Windows\System32\kurto.exe
```

### Linux / macOS

#### Homebrew (macOS / Linux)

```shell
brew install kurto
```

#### Binary download

| Platform | Download |
|---|---|
| Linux amd64 | `kurto-linux-amd64.gz` |
| Linux arm64 | `kurto-linux-arm64.gz` |
| Linux arm | `kurto-linux-arm.gz` |
| Linux 386 | `kurto-linux-386.gz` |
| Windows amd64 | `kurto-windows-amd64.zip` |
| Windows 386 | `kurto-windows-386.zip` |
| macOS amd64 | `kurto-darwin-amd64.gz` |
| macOS arm64 | `kurto-darwin-arm64.gz` |

```shell
# Linux / macOS
gunzip kurto-linux-amd64.gz
chmod +x kurto-linux-amd64
sudo mv kurto-linux-amd64 /usr/local/bin/kurto
```

#### From source

```shell
go install github.com/SomeoneUnlicensed/kurto@latest
```

## Quickstart

```shell
# Pull an image (Docker Hub V2 API)
kurto pull alpine

# Run a container
kurto run alpine -- echo hello

# Run with resource limits
kurto run --memory 256m --cpus 0.5 nginx

# Auto-remove on exit
kurto run --rm alpine -- echo done

# Name your container
kurto run --name myapp alpine sh
```

## Commands

### Containers

| Command | Shorthand | Description |
|---|---|---|
| `kurto run` | `r` | Create and start a container |
| `kurto ps` | | List containers (`-a` for all, `-n` namespace, `-l` label filter) |
| `kurto stop` | `st` | Stop a running container |
| `kurto rm` | | Remove a container |
| `kurto exec` | `e` | Execute a command in a container |
| `kurto logs` | `l` | Show container logs |

### Images

| Command | Shorthand | Description |
|---|---|---|
| `kurto pull` | `pu` | Pull image from Docker Hub |
| `kurto images` | `im` | List local images |

### Pods (beta)

| Command | Description |
|---|---|
| `kurto pod run -f file.yaml` | Create pod from YAML |
| `kurto pod ps` | List pods |
| `kurto pod stop` | Stop a pod |
| `kurto pod rm` | Remove a pod |

### Kubernetes-style

| Command | Shorthand | Description |
|---|---|---|
| `kurto apply -f file.yaml` | `a` | Apply YAML resources |
| `kurto get pods|containers|images` | `g` | List resources |
| `kurto get pod|container <name>` | `g` | Describe resource |
| `kurto delete pod|container|image <name>` | `d` | Delete resource |

### Run flags

```
kurto run [--name name] [--memory 256m] [--cpus 0.5] [--rm] <image> [-- cmd...]
```

## YAML example

```yaml
apiVersion: kurto.io/v1
kind: Pod
metadata:
  name: web
  labels:
    app: web
spec:
  containers:
  - name: nginx
    image: nginx:latest
    resources:
      memory: "256m"
      cpus: "0.5"
    ports:
    - containerPort: 80
  - name: sidecar
    image: alpine
    command: ["tail", "-f", "/dev/null"]
```

## How it works

| Platform | Isolation |
|---|---|
| Linux | PID, mount, UTS, IPC, NET namespaces; cgroups v2; chroot |
| Windows | Job Objects (process limits, memory quota, kill-on-close) |
| macOS | Process isolation via setpgid |

All platforms share the same CLI, state format, image pulling, and YAML resource model.

### Shell (WSL-like)

```shell
kurto shell        # Open interactive Alpine shell
kurto s            # Same, short alias
kurto s ubuntu     # Shell into Ubuntu container
kurto wsl          # WSL-compatible alias
```

On Windows, `kurto shell` detects Windows Terminal and opens a new tab running the container. Falls back to a new cmd.exe window if Windows Terminal is not available.

### What does `kurto r alpine .` do?

It runs image `alpine` with command `.` (dot). In Linux, `.` is a shell builtin that sources a file. Since kurto's default shell is `/bin/sh`, passing `.` as a command will fail with `exec: ".": executable file not found`. Use explicit shell:

```shell
kurto r alpine -- /bin/sh -c '. /etc/os-release && echo $NAME'
```

Without `--`, the first argument after the image name is treated as the command.

## How it works on Windows

### Job Objects

Each kurto container on Windows is a Windows Job Object. The process is created inside the job, which provides:

- **Process isolation** — the job is configured with `KILL_ON_JOB_CLOSE`: when the parent kurto process exits, all container processes are terminated automatically.
- **Active process limit** — limited to 1 process by default (the main command).
- **Memory limit** — if `--memory` is specified, `JOBOBJECT_LIMIT_JOB_MEMORY` and `JOBOBJECT_LIMIT_PROCESS_MEMORY` are set on the job.

The job handle is persisted to `containers/<id>/job` and cleaned up when the container exits.

### Process creation

The container command is started via `os/exec` with `CREATE_NEW_PROCESS_GROUP`, then assigned to the job object via `AssignProcessToJobObject`. The PID is saved to `containers/<id>/pid`.

### Filesystem isolation

Each container gets its own rootfs at `containers/<id>/rootfs/`. The image is extracted via tar.gz. The process uses this as its working directory.

### Environment isolation

Container-specific environment variables (`KURTO_CONTAINER`, `KURTO_ROOTFS`, user `--env` vars) are injected. The process does not inherit the full parent environment.

### Log capture

stdout and stderr are captured via `io.MultiWriter`: output goes to the terminal and to `containers/<id>/logs`. Enables `kurto logs <container>`.

### Limitations on Windows

- No network namespace isolation (containers share host networking)
- No filesystem namespace isolation (chroot is Linux-only; uses working directory isolation)
- Image format is a single tar.gz (no OCI layer merging)
- `whoami` and some system commands may fail due to the job object environment

## Build from source

```shell
git clone https://github.com/SomeoneUnlicensed/kurto
cd kurto
go build -o kurto .
```

## License

MIT
