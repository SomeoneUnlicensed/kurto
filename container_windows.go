//go:build windows

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
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procCreateJobObject    = modkernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject = modkernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = modkernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject = modkernel32.NewProc("TerminateJobObject")
	procQueryInformationJobObject = modkernel32.NewProc("QueryInformationJobObject")
)

const (
	JobObjectBasicLimitInformation   = 2
	JobObjectExtendedLimitInformation = 9
	JobObjectLimitActiveProcess      = 0x00000008
	JobObjectLimitProcessMemory      = 0x00000100
	JobObjectLimitJobMemory          = 0x00000200
	JobObjectLimitKillOnJobClose     = 0x00002000
)

type JOBOBJECT_BASIC_LIMIT_INFORMATION struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type JOBOBJECT_EXTENDED_LIMIT_INFORMATION struct {
	BasicInfo       JOBOBJECT_BASIC_LIMIT_INFORMATION
	IoInfo          struct {
		ReadOperationCount  int64
		WriteOperationCount int64
		OtherOperationCount int64
		ReadTransferCount   int64
		WriteTransferCount  int64
		OtherTransferCount  int64
	}
	ProcessMemoryLimit uintptr
	JobMemoryLimit     uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed  uintptr
}

func platformStart(c *Container) (int, error) {
	rootfs := filepath.Join(ContainersDir, c.ID, "rootfs")

	cmd := c.Config.Command
	if len(cmd) == 0 {
		cmd = []string{"cmd.exe", "/c", "echo", "No command specified"}
	}

	// Create Job Object
	jobName := "kurto_" + c.ID
	jobNamePtr, _ := windows.UTF16PtrFromString(jobName)
	jobHandle, _, err := procCreateJobObject.Call(0, uintptr(unsafe.Pointer(jobNamePtr)))
	if jobHandle == 0 {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}

	// Set kill on close so job dies when handle is closed
	var basicInfo JOBOBJECT_BASIC_LIMIT_INFORMATION
	basicInfo.LimitFlags = JobObjectLimitKillOnJobClose | JobObjectLimitActiveProcess
	basicInfo.ActiveProcessLimit = 1

	if c.Config.Resources.Memory != "" {
		memBytes := parseMemoryBytesWin(c.Config.Resources.Memory)
		if memBytes > 0 {
			var extInfo JOBOBJECT_EXTENDED_LIMIT_INFORMATION
			extInfo.BasicInfo = basicInfo
			extInfo.BasicInfo.LimitFlags |= JobObjectLimitJobMemory | JobObjectLimitProcessMemory
			extInfo.JobMemoryLimit = uintptr(memBytes)
			extInfo.ProcessMemoryLimit = uintptr(memBytes)
			procSetInformationJobObject.Call(jobHandle, JobObjectExtendedLimitInformation,
				uintptr(unsafe.Pointer(&extInfo)), unsafe.Sizeof(extInfo))
		}
	} else {
		procSetInformationJobObject.Call(jobHandle, JobObjectBasicLimitInformation,
			uintptr(unsafe.Pointer(&basicInfo)), unsafe.Sizeof(basicInfo))
	}

	env := os.Environ()
	env = append(env, "KURTO_CONTAINER="+c.ID)
	env = append(env, "KURTO_ROOTFS="+rootfs)
	for _, e := range c.Config.Env {
		env = append(env, e.Name+"="+e.Value)
	}

	// Build command line (Windows requires single string for CreateProcess)
	args := make([]string, len(cmd))
	copy(args, cmd)
	if len(args) > 0 {
		if resolved, err := exec.LookPath(args[0]); err == nil {
			args[0] = resolved
		}
	}

	// Set up command with log capture
	execCmd := exec.Command(args[0], args[1:]...)
	execCmd.Env = env
	execCmd.Dir = rootfs
	execCmd.Stdin = os.Stdin

	logFile, _ := os.Create(filepath.Join(ContainersDir, c.ID, "logs"))
	if logFile != nil {
		execCmd.Stdout = io.MultiWriter(os.Stdout, logFile)
		execCmd.Stderr = io.MultiWriter(os.Stderr, logFile)
	} else {
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
	}

	// Set process creation flags for isolation
	if execCmd.SysProcAttr == nil {
		execCmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	execCmd.SysProcAttr.CreationFlags = windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_UNICODE_ENVIRONMENT
	execCmd.SysProcAttr.HideWindow = false

	// Start the process
	if err := execCmd.Start(); err != nil {
		return 0, fmt.Errorf("start process: %w", err)
	}

	// Assign to job object via OpenProcess
	procHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(execCmd.Process.Pid))
	if err == nil && procHandle != 0 {
		procAssignProcessToJobObject.Call(jobHandle, uintptr(procHandle))
		windows.CloseHandle(procHandle)
	}

	pidFile := filepath.Join(ContainersDir, c.ID, "pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", execCmd.Process.Pid)), 0644)

	// Store job handle for later use
	jobFile := filepath.Join(ContainersDir, c.ID, "job")
	os.WriteFile(jobFile, []byte(fmt.Sprintf("%d", jobHandle)), 0644)

	// Monitor in background
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
		// Close handles
		if logFile != nil {
			logFile.Close()
		}
		windows.CloseHandle(windows.Handle(jobHandle))
		if c.Config.AutoRemove {
			os.RemoveAll(filepath.Join(ContainersDir, c.ID))
		}
	}()

	return execCmd.Process.Pid, nil
}

func platformStop(c *Container) error {
	// Try graceful stop via Ctrl-Break first, then terminate job
	pidFile := filepath.Join(ContainersDir, c.ID, "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("container %s not running", c.ID)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))

	if pid > 0 {
		// Generate Ctrl-Break for the process group
		proc, err := os.FindProcess(pid)
		if err == nil {
			proc.Signal(os.Kill) // Windows only supports Kill
			done := make(chan struct{})
			go func() {
				proc.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				// Terminate job object
				jobFile := filepath.Join(ContainersDir, c.ID, "job")
				if jobData, err := os.ReadFile(jobFile); err == nil {
					if jh, err := strconv.ParseUint(strings.TrimSpace(string(jobData)), 10, 64); err == nil {
						procTerminateJobObject.Call(uintptr(jh), 1)
					}
				}
			}
		}
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
	// Close job handle
	jobFile := filepath.Join(ContainersDir, c.ID, "job")
	if data, err := os.ReadFile(jobFile); err == nil {
		if jh, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			windows.CloseHandle(windows.Handle(jh))
		}
	}
	os.Remove(jobFile)
	return nil
}

func handleNsenter() {
	os.Exit(0) // __nsenter__ is Linux-only
}

func parseMemoryBytesWin(s string) uint64 {
	s = strings.ToLower(strings.TrimSpace(s))
	var mult uint64 = 1
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
	v, _ := strconv.ParseUint(s, 10, 64)
	return v * mult
}
