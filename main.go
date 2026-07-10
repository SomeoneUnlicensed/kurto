package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var cmds = map[string]string{
	"r": "run", "run":     "run",
	"ps":    "ps",
	"im":   "images", "images": "images", "img": "images",
	"pu":   "pull", "pull":   "pull",
	"st":   "stop", "stop":   "stop",
	"rm":   "rm",
	"e":    "exec", "exec":   "exec",
	"l":    "logs", "logs":   "logs",
	"p":    "pod", "pod":     "pod", "pods": "pod",
	"a":    "apply", "apply": "apply",
	"g":    "get", "get":     "get", "describe": "get",
	"d":    "delete", "delete": "delete", "del": "delete",
}

var helpText = `KURTO — Unikorn Container Runtime
Cross-platform (Linux + Windows), K8s-style, zero-config.

USAGE:
  kurto <command> [flags] [args]

COMMANDS:
  Container:
    r,   run     [--name n] [--memory m] [--cpus c] [--rm] <image> [-- cmd...]
    ps          [-a] [-n ns] [-l k=v]
    st,  stop   <container>...
    rm          <container>...
    e,   exec   <container> <cmd...>
    l,   logs   <container>

  Images:
    im,  images
    pu,  pull   <image>[:tag]

  Pods (k8s-style):
    p,   pod    run -f file.yaml
    p,   pod    ps [-n ns] [-l k=v]
    p,   pod    stop <pod>
    p,   pod    rm   <pod>

  Kubernetes-style:
    a,   apply  -f file.yaml
    g,   get    pods|containers|images [-n ns] [-l k=v]
    g,   get    pod|container <name>   (describe)
    d,   delete pod|container|image <name>

EXAMPLES:
  kurto r alpine
  kurto r --rm --name myapp alpine echo hello
  kurto r --memory 256m --cpus 0.5 nginx
  kurto ps -a
  kurto pu nginx:latest
  kurto e myapp sh
  kurto g pods
  kurto a -f pod.yaml
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(helpText)
		return
	}

	cmd := os.Args[1]

	if cmd == "--help" || cmd == "-h" || cmd == "help" {
		fmt.Print(helpText)
		return
	}

	if cmd == "__nsenter__" {
		handleNsenter()
		return
	}

	// Resolve alias
	if resolved, ok := cmds[cmd]; ok {
		cmd = resolved
	} else {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		fmt.Fprintf(os.Stderr, "Try 'kurto --help'\n")
		os.Exit(1)
	}

	args := os.Args[2:]

	switch cmd {
	case "run":
		cmdRun(args)
	case "ps":
		cmdPS(args)
	case "images":
		cmdImages(args)
	case "pull":
		cmdPull(args)
	case "stop":
		cmdStop(args)
	case "rm":
		cmdRemove(args)
	case "exec":
		cmdExec(args)
	case "logs":
		cmdLogs(args)
	case "pod":
		cmdPod(args)
	case "apply":
		cmdApply(args)
	case "get":
		cmdGet(args)
	case "delete":
		cmdDelete(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

var boolFlags = map[string]bool{
	"rm": true,
	"a": true, "all": true,
}

func parseFlags(args []string) (flags map[string]string, positional []string) {
	flags = make(map[string]string)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		a = strings.TrimLeft(a, "-")

		// Handle --key=value
		if idx := strings.IndexByte(a, '='); idx >= 0 {
			flags[a[:idx]] = a[idx+1:]
			continue
		}

		if boolFlags[a] {
			flags[a] = "true"
			continue
		}

		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			flags[a] = args[i+1]
			i++
		} else {
			flags[a] = "true"
		}
	}
	return
}

// ─── run ─────────────────────────────────────────────────

func cmdRun(args []string) {
	flags, pos := parseFlags(args)

	name := flags["name"]
	memory := flags["m"] // -m or --memory
	if memory == "" {
		memory = flags["memory"]
	}
	cpus := flags["c"] // -c or --cpus
	if cpus == "" {
		cpus = flags["cpus"]
	}
	rm := flags["rm"] == "true"

	if len(pos) == 0 {
		fatal("usage: kurto run [flags] <image> [-- cmd...]")
	}

	image := pos[0]
	cmdArgs := pos[1:]

	if len(cmdArgs) == 0 {
		cmdArgs = []string{"/bin/sh"}
	}

	cfg := ContainerConfig{
		Name:      name,
		Image:     image,
		Command:   cmdArgs,
		Resources: ResourceSpec{Memory: memory, CPUs: cpus},
		AutoRemove: rm,
	}

	c, err := CreateContainer(cfg)
	if err != nil {
		fatalf("create: %v", err)
	}
	fmt.Printf("Created %s (%s)\n", c.Name, c.ID[:12])

	if err := StartContainer(c.ID); err != nil {
		fatalf("start: %v", err)
	}

	store := NewStore()
	ct, _ := store.LoadContainer(c.ID)
	if ct != nil && ct.PID > 0 {
		fmt.Printf("Started %s (pid %d)\n", c.Name, ct.PID)
	} else {
		fmt.Printf("Started %s\n", c.Name)
	}

	// Wait for exit (poll with timeout)
	for i := 0; i < 300; i++ {
		ct, err := store.LoadContainer(c.ID)
		if err != nil || ct.Status == StatusExited || ct.Status == StatusStopped {
			if ct != nil && ct.ExitCode != 0 {
				fmt.Printf("Exited with code %d\n", ct.ExitCode)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ─── ps ───────────────────────────────────────────────────

func cmdPS(args []string) {
	flags, _ := parseFlags(args)
	all := flags["a"] == "true" || flags["all"] == "true"
	ns := flags["n"]
	if ns == "" {
		ns = flags["namespace"]
	}

	var sel map[string]string
	if l := flags["l"]; l != "" {
		sel = parseLabelSelector(l)
	}

	ListContainers(ns, all, sel)
}

// ─── images ───────────────────────────────────────────────

func cmdImages(args []string) {
	ListImages()
}

// ─── pull ─────────────────────────────────────────────────

func cmdPull(args []string) {
	_, pos := parseFlags(args)
	if len(pos) < 1 {
		fatal("usage: kurto pull <image>[:tag]")
	}
	if err := PullImage(pos[0]); err != nil {
		fatalf("pull: %v", err)
	}
}

// ─── stop ─────────────────────────────────────────────────

func cmdStop(args []string) {
	if len(args) < 1 {
		fatal("usage: kurto stop <container>...")
	}
	for _, ref := range args {
		c, err := NewStore().FindContainer(ref)
		if err != nil {
			fatalf("stop: %v", err)
		}
		if err := StopContainer(c.ID); err != nil {
			fatalf("stop %s: %v", ref, err)
		}
		fmt.Printf("Stopped %s\n", c.Name)
	}
}

// ─── rm ───────────────────────────────────────────────────

func cmdRemove(args []string) {
	if len(args) < 1 {
		fatal("usage: kurto rm <container>...")
	}
	for _, ref := range args {
		c, err := NewStore().FindContainer(ref)
		if err != nil {
			fatalf("rm: %v", err)
		}
		if err := RemoveContainer(c.ID); err != nil {
			fatalf("rm %s: %v", ref, err)
		}
		fmt.Printf("Removed %s\n", c.Name)
	}
}

// ─── exec ─────────────────────────────────────────────────

func cmdExec(args []string) {
	if len(args) < 2 {
		fatal("usage: kurto exec <container> <cmd...>")
	}
	c, err := NewStore().FindContainer(args[0])
	if err != nil {
		fatalf("exec: %v", err)
	}
	if err := platformExec(c, args[1:]); err != nil {
		fatalf("exec: %v", err)
	}
}

// ─── logs ─────────────────────────────────────────────────

func cmdLogs(args []string) {
	if len(args) < 1 {
		fatal("usage: kurto logs <container>")
	}
	c, err := NewStore().FindContainer(args[0])
	if err != nil {
		fatalf("logs: %v", err)
	}
	if err := LogsContainer(c.ID); err != nil {
		fatalf("logs: %v", err)
	}
}

// ─── pod ──────────────────────────────────────────────────

func cmdPod(args []string) {
	if len(args) < 1 {
		fatal("usage: kurto pod <run|ps|stop|rm> ...")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "run":
		flags, _ := parseFlags(subArgs)
		file := flags["f"]
		if file == "" {
			fatal("usage: kurto pod run -f file.yaml")
		}
		if err := ApplyFile(file); err != nil {
			fatalf("pod run: %v", err)
		}
	case "ps", "ls":
		flags, _ := parseFlags(subArgs)
		ns := flags["n"]
		var sel map[string]string
		if l := flags["l"]; l != "" {
			sel = parseLabelSelector(l)
		}
		ListPods(ns, sel)
	case "stop":
		for _, ref := range subArgs {
			if err := StopPod(ref); err != nil {
				fatalf("pod stop: %v", err)
			}
			fmt.Printf("Pod %s stopped\n", ref)
		}
	case "rm", "delete":
		for _, ref := range subArgs {
			if err := RemovePod(ref); err != nil {
				fatalf("pod rm: %v", err)
			}
			fmt.Printf("Pod %s removed\n", ref)
		}
	default:
		fatalf("unknown pod subcommand: %s", sub)
	}
}

// ─── apply ────────────────────────────────────────────────

func cmdApply(args []string) {
	flags, _ := parseFlags(args)
	file := flags["f"]
	if file == "" {
		fatal("usage: kurto apply -f file.yaml")
	}
	if err := ApplyFile(file); err != nil {
		fatalf("apply: %v", err)
	}
	fmt.Printf("Applied %s\n", file)
}

// ─── get ──────────────────────────────────────────────────

func cmdGet(args []string) {
	flags, pos := parseFlags(args)
	ns := flags["n"]
	if ns == "" {
		ns = flags["namespace"]
	}
	var sel map[string]string
	if l := flags["l"]; l != "" {
		sel = parseLabelSelector(l)
	}

	if len(pos) == 0 {
		fatal("usage: kurto get pods|containers|images [name]")
	}

	kind := pos[0]
	if len(pos) >= 2 {
		// Describe specific resource
		DescribeResource(kind, pos[1])
	} else {
		GetResources(kind, ns, sel)
	}
}

// ─── delete ───────────────────────────────────────────────

func cmdDelete(args []string) {
	if len(args) < 2 {
		fatal("usage: kurto delete <pod|container|image> <name>")
	}
	if err := DeleteResource(args[0], args[1]); err != nil {
		fatalf("delete: %v", err)
	}
	fmt.Printf("Deleted %s %s\n", args[0], args[1])
}

// ─── helpers ──────────────────────────────────────────────

func parseLabelSelector(s string) map[string]string {
	sel := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
			sel[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return sel
}

func fatal(v ...interface{}) {
	fmt.Fprintln(os.Stderr, "error:", fmt.Sprint(v...))
	os.Exit(1)
}

func fatalf(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", v...)
	os.Exit(1)
}
