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

### Binary release

Download the latest release for your platform from the [Releases page](https://github.com/SomeoneUnlicensed/kurto/releases).

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

# Windows (PowerShell)
Expand-Archive kurto-windows-amd64.zip -DestinationPath .
mv kurto-windows-amd64.exe kurto.exe
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

## Build from source

```shell
git clone https://github.com/SomeoneUnlicensed/kurto
cd kurto
go build -o kurto .
```

## License

MIT
