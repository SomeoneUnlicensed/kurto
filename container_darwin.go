//go:build darwin

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func platformStart(c *Container) (int, error) {
	rootfs := filepath.Join(ContainersDir, c.ID, "rootfs")

	cmd := c.Config.Command
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/root",
		"USER=root",
		"TERM=xterm-256color",
		"KURTO_CONTAINER=" + c.ID,
	}
	for _, e := range c.Config.Env {
		env = append(env, e.Name+"="+e.Value)
	}

	ec := exec.Command(cmd[0], cmd[1:]...)
	ec.Env = env
	ec.Dir = rootfs
	ec.Stdin = os.Stdin

	logFile, _ := os.Create(filepath.Join(ContainersDir, c.ID, "logs"))
	if logFile != nil {
		ec.Stdout = io.MultiWriter(os.Stdout, logFile)
		ec.Stderr = io.MultiWriter(os.Stderr, logFile)
	} else {
		ec.Stdout = os.Stdout
		ec.Stderr = os.Stderr
	}

	ec.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := ec.Start(); err != nil {
		return 0, fmt.Errorf("start: %w", err)
	}

	pidFile := filepath.Join(ContainersDir, c.ID, "pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", ec.Process.Pid)), 0644)

	go func() {
		err := ec.Wait()
		exitCode := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			}
		}
		if logFile != nil {
			logFile.Close()
		}
		store := NewStore()
		if ct, _ := store.LoadContainer(c.ID); ct != nil {
			ct.Status = StatusExited
			ct.ExitCode = exitCode
			store.SaveContainer(ct)
		}
		if c.Config.AutoRemove {
			os.RemoveAll(filepath.Join(ContainersDir, c.ID))
		}
	}()

	return ec.Process.Pid, nil
}

func platformStop(c *Container) error {
	pidFile := filepath.Join(ContainersDir, c.ID, "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("container %s not running", c.ID)
	}
	pid := 0
	fmt.Sscanf(string(data), "%d", &pid)
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
	case <-time.After(5 * time.Second):
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
	ec.Dir = rootfs
	return ec.Run()
}

func platformCleanup(c *Container) error {
	return nil
}

func handleNsenter() {
	os.Exit(0)
}
