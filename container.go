package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func CreateContainer(cfg ContainerConfig) (*Container, error) {
	id := NewID()
	if cfg.Name == "" {
		cfg.Name = id[:12]
	}
	if cfg.Namespace == "" {
		cfg.Namespace = DefaultNS
	}

	// Auto-pull if image doesn't exist locally
	store := NewStore()
	imgName, imgTag := cfg.Image, DefaultTag
	if i := lastIndexByte(cfg.Image, ':'); i >= 0 {
		imgName = cfg.Image[:i]
		imgTag = cfg.Image[i+1:]
	}
	if _, err := store.LoadImage(imgName, imgTag); err != nil {
		if err := PullImage(cfg.Image); err != nil {
			return nil, fmt.Errorf("pull %s: %w", cfg.Image, err)
		}
	}

	// Extract image to container rootfs
	rootfs := filepath.Join(ContainersDir, id, "rootfs")
	img, err := store.LoadImage(imgName, imgTag)
	if err != nil {
		return nil, fmt.Errorf("image %s:%s: %w", imgName, imgTag, err)
	}
	if err := extractImage(img, rootfs); err != nil {
		return nil, fmt.Errorf("extract rootfs: %w", err)
	}

	c := &Container{
		ID:      id,
		Name:    cfg.Name,
		Image:   cfg.Image,
		Status:  StatusCreated,
		Created: time.Now(),
		Config:  cfg,
		PlatformOS: runtime.GOOS,
	}
	if err := store.SaveContainer(c); err != nil {
		return nil, err
	}
	return c, nil
}

func StartContainer(id string) error {
	store := NewStore()
	c, err := store.LoadContainer(id)
	if err != nil {
		return fmt.Errorf("container %s: %w", id, err)
	}
	if c.Status == StatusRunning {
		return fmt.Errorf("container %s already running", id)
	}

	// Platform-specific start
	pid, err := platformStart(c)
	if err != nil {
		c.Status = StatusExited
		store.SaveContainer(c)
		return err
	}

	c.PID = pid
	c.Status = StatusRunning
	return store.SaveContainer(c)
}

func StopContainer(id string) error {
	store := NewStore()
	c, err := store.LoadContainer(id)
	if err != nil {
		return err
	}
	if c.Status != StatusRunning {
		return fmt.Errorf("container %s is not running", id)
	}
	return platformStop(c)
}

func RemoveContainer(id string) error {
	store := NewStore()
	c, err := store.LoadContainer(id)
	if err != nil {
		return fmt.Errorf("container %s: %w", id, err)
	}
	if c.Status == StatusRunning {
		return fmt.Errorf("cannot remove running container %s", id)
	}
	// Clean up rootfs
	rootfs := filepath.Join(ContainersDir, c.ID, "rootfs")
	if err := platformCleanup(c); err != nil {
		// non-fatal
	}
	os.RemoveAll(rootfs)
	return store.RemoveContainer(id)
}

func ListContainers(namespace string, all bool, labelSelector map[string]string) error {
	store := NewStore()
	containers, err := store.ListContainers(namespace)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		fmt.Println("No containers")
		return nil
	}
	header := fmt.Sprintf("%-12s %-20s %-20s %-10s %-8s %-20s", "ID", "NAME", "IMAGE", "STATUS", "PID", "CREATED")
	fmt.Println(header)
	for _, c := range containers {
		if !all && c.Status != StatusRunning {
			continue
		}
		if labelSelector != nil && !matchLabels(c.Config.Labels, labelSelector) {
			continue
		}
		pid := ""
		if c.PID > 0 {
			pid = fmt.Sprintf("%d", c.PID)
		}
		fmt.Printf("%-12s %-20s %-20s %-10s %-8s %-20s\n",
			c.ID[:12], c.Name, c.Image, c.Status, pid, c.Created.Format(time.Stamp))
	}
	return nil
}

func LogsContainer(id string) error {
	logPath := filepath.Join(ContainersDir, id, "logs")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("no logs for container %s", id)
	}
	fmt.Print(string(data))
	return nil
}

func WriteContainerLog(id string, data []byte) error {
	logPath := filepath.Join(ContainersDir, id, "logs")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
