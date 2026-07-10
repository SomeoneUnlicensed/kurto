//go:build linux

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func platformStart(c *Container) (int, error) {
	rootfs := filepath.Join(ContainersDir, c.ID, "rootfs")
	ensureDirs(rootfs, "proc", "sys", "dev", "tmp")
	rootfs, _ = filepath.Abs(rootfs)

	cmd := c.Config.Command
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}
	args := cmd
	if binary, err := exec.LookPath(args[0]); err == nil {
		args[0] = binary
	}

	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root", "USER=root", "TERM=xterm-256color",
	}
	for _, e := range c.Config.Env {
		env = append(env, e.Name+"="+e.Value)
	}

	execCmd := exec.Command("/proc/self/exe", append([]string{
		"__nsenter__", c.ID, c.Config.Resources.Memory, c.Config.Resources.CPUs,
	}, args...)...)
	execCmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID | syscall.CLONE_NEWIPC | syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
		Chroot:       rootfs,
	}
	execCmd.Dir = "/"
	execCmd.Env = env
	logFile, _ := os.Create(filepath.Join(ContainersDir, c.ID, "logs"))
	if logFile != nil {
		execCmd.Stdout = io.MultiWriter(os.Stdout, logFile)
		execCmd.Stderr = io.MultiWriter(os.Stderr, logFile)
	} else {
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
	}
	execCmd.Stdin = os.Stdin

	if err := execCmd.Start(); err != nil {
		return 0, fmt.Errorf("start: %w", err)
	}

	pidFile := filepath.Join(ContainersDir, c.ID, "pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", execCmd.Process.Pid)), 0644)

	go func() {
		err := execCmd.Wait()
		exitCode := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			}
		}
		store := NewStore()
		if ct, _ := store.LoadContainer(c.ID); ct != nil {
			ct.Status = StatusExited
			ct.ExitCode = exitCode
			store.SaveContainer(ct)
		}
		if logFile != nil {
			logFile.Close()
		}
		if c.Config.AutoRemove {
			os.RemoveAll(filepath.Join(ContainersDir, c.ID))
		}
	}()

	return execCmd.Process.Pid, nil
}

func platformStop(c *Container) error {
	pidFile := filepath.Join(ContainersDir, c.ID, "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("container %s not running", c.ID)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	proc.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		proc.Kill()
	}
	c.Status = StatusStopped
	return nil
}

func platformExec(c *Container, cmd []string) error {
	rootfs := filepath.Join(ContainersDir, c.ID, "rootfs")
	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		return fmt.Errorf("container %s not found", c.ID)
	}
	ec := exec.Command(cmd[0], cmd[1:]...)
	ec.Stdin = os.Stdin
	ec.Stdout = os.Stdout
	ec.Stderr = os.Stderr
	ec.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID | syscall.CLONE_NEWNET,
		Chroot: rootfs,
	}
	ec.Dir = "/"
	return ec.Run()
}

func platformCleanup(c *Container) error {
	rootfs := filepath.Join(ContainersDir, c.ID, "rootfs")
	for _, d := range []string{"proc", "sys", "dev"} {
		syscall.Unmount(filepath.Join(rootfs, d), 0)
	}
	return nil
}

func ensureDirs(root string, dirs ...string) {
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0755)
	}
}

func handleNsenter() { nsenterChild() }

func nsenterChild() {
	if len(os.Args) < 5 {
		os.Exit(1)
	}
	containerID := os.Args[2]
	memoryLimit := os.Args[3]
	cpuLimit := os.Args[4]
	cmd := os.Args[5:]
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	rootfs := filepath.Join(ContainersDir, containerID, "rootfs")
	syscall.Mount("proc", filepath.Join(rootfs, "proc"), "proc", 0, "")
	syscall.Mount("sysfs", filepath.Join(rootfs, "sys"), "sysfs", 0, "")
	syscall.Mount("tmpfs", filepath.Join(rootfs, "dev"), "tmpfs", 0, "mode=755")
	syscall.Sethostname([]byte(containerID[:12]))
	setupCgroupsV2(containerID, memoryLimit, cpuLimit)

	os.Chdir("/")
	if err := syscall.Exec(cmd[0], cmd, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec: %v\n", err)
		os.Exit(1)
	}
}

func setupCgroupsV2(containerID, memory, cpus string) {
	cgPath := "/sys/fs/cgroup/kurto/" + containerID
	os.MkdirAll(cgPath, 0755)

	if memory != "" {
		val := parseMemoryBytes(memory)
		if val > 0 {
			os.WriteFile(cgPath+"/memory.max", []byte(fmt.Sprintf("%d\n", val)), 0644)
		}
	}
	if cpus != "" {
		quota := int(parseCPUs(cpus) * 100000)
		if quota > 0 {
			os.WriteFile(cgPath+"/cpu.max", []byte(fmt.Sprintf("%d 100000\n", quota)), 0644)
		}
	}
	os.WriteFile(cgPath+"/cgroup.procs", []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
}

func parseMemoryBytes(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	var mult int64 = 1
	switch {
	case strings.HasSuffix(s, "g"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		mult = 1 << 10
		s = strings.TrimSuffix(s, "k")
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v * mult
}

func parseCPUs(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
