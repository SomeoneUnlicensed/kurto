package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repoOwner = "SomeoneUnlicensed"
const repoName = "kurto"

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// ─── Deploy ──────────────────────────────────────────────

func cmdDeploy(args []string) {
	if len(args) < 1 {
		fatal("usage: kurto deploy [--user root] [--port 22] [--key path] [--target-os windows] [--manifest dir] <host>")
	}

	user := "root"
	port := "22"
	key := ""
	targetOS := ""
	manifestDir := "."
	host := ""

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--user" && i+1 < len(args):
			user = args[i+1]
			i++
		case args[i] == "--port" && i+1 < len(args):
			port = args[i+1]
			i++
		case args[i] == "--key" && i+1 < len(args):
			key = args[i+1]
			i++
		case args[i] == "--target-os" && i+1 < len(args):
			targetOS = args[i+1]
			i++
		case args[i] == "--manifest" && i+1 < len(args):
			manifestDir = args[i+1]
			i++
		default:
			host = args[i]
		}
	}

	if host == "" {
		fatal("usage: kurto deploy [flags] <host>")
	}

	sshTarget := fmt.Sprintf("%s@%s", user, host)

	fmt.Printf("Deploying to %s...\n", sshTarget)

	// Auto-detect target OS if not specified
	if targetOS == "" {
		out, _ := sshOutput(sshTarget, port, key, "uname -s 2>/dev/null || echo Windows")
		out = strings.TrimSpace(out)
		switch {
		case strings.Contains(out, "Linux"):
			targetOS = "linux"
		case strings.Contains(out, "Darwin"):
			targetOS = "darwin"
		default:
			targetOS = "windows"
		}
		fmt.Printf("Detected target OS: %s\n", targetOS)
	}

	targetArch := "amd64"
	out, _ := sshOutput(sshTarget, port, key, "uname -m 2>/dev/null || echo AMD64")
	out = strings.TrimSpace(out)
	switch {
	case strings.Contains(out, "aarch64"), strings.Contains(out, "arm64"):
		targetArch = "arm64"
	case strings.Contains(out, "arm"):
		targetArch = "arm"
	case strings.Contains(out, "386"), strings.Contains(out, "i386"):
		targetArch = "386"
	}

	tmpDir, _ := os.MkdirTemp("", "kurto-deploy")
	defer os.RemoveAll(tmpDir)

	ext := ""
	if targetOS == "windows" {
		ext = ".exe"
	}
	targetBinary := filepath.Join(tmpDir, "kurto"+ext)

	fmt.Printf("Building kurto for %s/%s...", targetOS, targetArch)
	build := exec.Command("go", "build", "-ldflags=-X main.version="+version, "-o", targetBinary, ".")
	build.Env = append(os.Environ(), "GOOS="+targetOS, "GOARCH="+targetArch)
	if out, err := build.CombinedOutput(); err != nil {
		fatalf("build: %v\n%s", err, out)
	}
	fmt.Println(" done")

	if targetOS == "windows" {
		deployWindows(sshTarget, port, key, targetBinary, manifestDir)
	} else {
		deployUnix(sshTarget, port, key, targetBinary, manifestDir)
	}

	// Status
	fmt.Print("Checking status...")
	statusCmd := "kurto ps -a 2>/dev/null || kurto.exe ps -a"
	out2, _ := sshOutput(sshTarget, port, key, statusCmd)
	fmt.Println("\n" + strings.TrimSpace(out2))
}

func deployUnix(target, port, key, binary, manifestDir string) {
	fmt.Print("Copying kurto to server...")
	scpRun(target, port, key, binary, "/tmp/kurto")
	fmt.Println(" done")

	fmt.Print("Installing on server...")
	sshRun(target, port, key, "sudo mv /tmp/kurto /usr/local/bin/kurto && sudo chmod +x /usr/local/bin/kurto")
	fmt.Println(" done")

	deployManifests(target, port, key, manifestDir, "/etc/kurto/manifests", "", "")
}

func deployWindows(target, port, key, binary, manifestDir string) {
	remoteDir := "C:\\ProgramData\\Kurto"

	fmt.Print("Creating remote directory...")
	sshRun(target, port, key, "mkdir "+remoteDir+" 2>nul")
	fmt.Println(" done")

	fmt.Print("Copying kurto to server...")
	scpRun(target, port, key, binary, remoteDir+"\\kurto.exe")
	fmt.Println(" done")

	fmt.Print("Adding to PATH...")
	sshRun(target, port, key,
		`setx /M PATH "%PATH%;`+remoteDir+`" 2>nul || echo already in PATH`)
	fmt.Println(" done")

	deployManifests(target, port, key, manifestDir, remoteDir+"\\manifests", ".yaml", remoteDir+"\\")
}

func deployManifests(target, port, key, localDir, remoteDir, extFilter, prefix string) {
	files := findManifests(localDir)
	if len(files) == 0 {
		return
	}

	fmt.Print("Copying manifests...")
	sshRun(target, port, key, "mkdir "+remoteDir+" 2>nul")
	for _, f := range files {
		rem := remoteDir + "/" + filepath.Base(f)
		scpRun(target, port, key, f, rem)
	}
	fmt.Println(" done")

	fmt.Print("Applying manifests...")
	applyCmd := prefix + "kurto apply -f " + remoteDir + "/*" + extFilter
	sshRun(target, port, key, applyCmd)
	fmt.Println(" done")
}

func buildSSHArgs(port, key string) []string {
	args := []string{"-p", port, "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10"}
	if key != "" {
		args = append(args, "-i", key)
	}
	return args
}

func scpRun(target, port, key, src, dst string) {
	args := buildSSHArgs(port, key)
	args = append([]string{"-P", port, "-o", "StrictHostKeyChecking=no"}, src, target+":"+dst)
	if key != "" {
		args = append([]string{"-i", key}, args...)
	}
	exec.Command("scp", args...).Run()
}

func findManifests(dir string) []string {
	var files []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

func sshRun(target, port, key, cmd string) {
	args := buildSSHArgs(port, key)
	args = append(args, target, cmd)
	exec.Command("ssh", args...).Run()
}

func sshOutput(target, port, key, cmd string) (string, error) {
	args := buildSSHArgs(port, key)
	args = append(args, target, cmd)
	out, err := exec.Command("ssh", args...).Output()
	return string(out), err
}

// ─── Self-Update ─────────────────────────────────────────

func cmdSelfUpdate() {
	fmt.Println("Checking for updates...")
	release, err := fetchLatestRelease()
	if err != nil {
		fatalf("check update: %v", err)
	}

	if release.TagName == "v"+version || release.TagName == version {
		fmt.Printf("Already at latest version (%s)\n", version)
		return
	}

	fmt.Printf("New version available: %s (current: %s)\n", release.TagName, version)

	// Find the right asset for current platform
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	suffix := fmt.Sprintf("%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)

	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == "kurto-"+suffix {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		fatalf("no binary for %s/%s in release", runtime.GOOS, runtime.GOARCH)
	}

	exe, err := os.Executable()
	if err != nil {
		fatalf("executable path: %v", err)
	}
	exe, _ = filepath.EvalSymlinks(exe)

	tmpFile := exe + ".new"
	fmt.Printf("Downloading %s...\n", release.TagName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		fatalf("download: %v", err)
	}
	defer resp.Body.Close()

	f, err := os.Create(tmpFile)
	if err != nil {
		fatalf("create temp: %v", err)
	}

	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpFile)
		fatalf("write: %v", err)
	}

	if runtime.GOOS != "windows" {
		os.Chmod(tmpFile, 0755)
	}

	// Replace binary
	if runtime.GOOS == "windows" {
		// On Windows, write a batch script to rename after exit
		batchContent := fmt.Sprintf("@echo off\r\n"+
			":retry\r\n"+
			"del \"%s\"\r\n"+
			"if exist \"%s\" (\r\n"+
			"  timeout /t 1 /nobreak >nul\r\n"+
			"  goto retry\r\n"+
			")\r\n"+
			"rename \"%s\" \"%s\"\r\n"+
			"if exist \"%s.new\" (\r\n"+
			"  del \"%s.new\"\r\n"+
			")\r\n"+
			"echo Updated to %s\r\n",
			exe, exe, tmpFile, filepath.Base(exe), exe, exe, release.TagName)

		batchFile := exe + ".bat"
		os.WriteFile(batchFile, []byte(batchContent), 0644)
		fmt.Printf("Restart required. Run %s as Administrator to complete update.\n", batchFile)
		fmt.Println("Or manually replace the binary.")
	} else {
		os.Rename(tmpFile, exe)
		fmt.Printf("Updated to %s\n", release.TagName)
	}
}

// ─── Version Check ───────────────────────────────────────

func fetchLatestRelease() (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func checkVersionAsync() {
	go func() {
		// Only check once per day
		checkFile := filepath.Join(os.TempDir(), "kurto-version-check")
		if st, err := os.Stat(checkFile); err == nil {
			if time.Since(st.ModTime()) < 24*time.Hour {
				return
			}
		}

		release, err := fetchLatestRelease()
		if err != nil {
			return
		}

		tag := release.TagName
		if strings.HasPrefix(tag, "v") {
			tag = tag[1:]
		}
		cur := version
		if strings.HasPrefix(cur, "v") {
			cur = cur[1:]
		}

		if tag != cur && cur != "dev" {
			fmt.Fprintf(os.Stderr, "\nA new version of kurto is available: %s (current: %s)\n", release.TagName, version)
			fmt.Fprintf(os.Stderr, "Update with: kurto self-update\n\n")
		}

		os.WriteFile(checkFile, []byte(time.Now().String()), 0644)
	}()
}

func cmdVersionCheck() {
	release, err := fetchLatestRelease()
	if err != nil {
		fatalf("check version: %v", err)
	}

	fmt.Printf("Current:  %s\n", version)
	fmt.Printf("Latest:   %s\n", release.TagName)

	if release.TagName != "v"+version && release.TagName != version {
		fmt.Println("Status:   update available")
		fmt.Printf("Run:      kurto self-update\n")
	} else {
		fmt.Println("Status:   up to date")
	}
}
