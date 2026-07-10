# Contributing

## Structure

```
kurto/
  main.go                CLI entry point and command dispatch
  types.go               Shared types
  store.go               Local JSON state storage
  image.go               Image pulling (Docker Hub V2 API)
  container.go           Cross-platform container lifecycle
  container_linux.go     Linux implementation (namespaces + cgroups v2)
  container_windows.go   Windows implementation (Job Objects)
  container_darwin.go    macOS implementation (process isolation)
  pod.go                 Pod orchestration
  apply.go               YAML apply/get/delete
```

## Platform-specific code

Each platform file must export the same functions:

```go
func platformStart(c *Container) (int, error)
func platformStop(c *Container) error
func platformExec(c *Container, cmd []string) error
func platformCleanup(c *Container) error
func handleNsenter()
```

Use `//go:build linux`, `//go:build windows`, or `//go:build darwin` build tags. Only one platform file is compiled per target.

## Requirements

- Go 1.22 or later
- `go vet ./...` must pass
- Cross-compilation must work: `GOOS=linux GOARCH=amd64 go build ./...`

## Pull requests

1. Run `go vet ./...` before committing.
2. Verify cross-compilation for all platforms: `make dist`
3. Keep the CLI stable. Adding new commands is allowed, but do not break existing ones.
4. No external dependencies beyond the standard library, `golang.org/x/sys`, and `gopkg.in/yaml.v3`.

## Adding a new platform

1. Create `container_<os>.go` with `//go:build <os>`.
2. Implement the five required functions.
3. Add the target to `Makefile` and `.github/workflows/ci.yml`.

## Release process

```shell
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions builds and uploads all platform binaries automatically.
