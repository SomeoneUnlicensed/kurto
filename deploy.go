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
		fatal("usage: kurto deploy [--user root] [--port 22] [--key path] [--manifest dir] <host>")
	}

	user := "root"
	port := "22"
	key := ""
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
	sshArgs := []string{"-p", port, "-o", "StrictHostKeyChecking=no"}
	if key != "" {
		sshArgs = append(sshArgs, "-i", key)
	}

	fmt.Printf("Deploying to %s...\n", sshTarget)

	// Step 1: build binary for target (linux/amd64)
	tmpDir, _ := os.MkdirTemp("", "kurto-deploy")
	defer os.RemoveAll(tmpDir)

	targetBinary := filepath.Join(tmpDir, "kurto")
	if runtime.GOOS == "windows" {
		targetBinary += ".exe"
	}

	fmt.Print("Building kurto for linux/amd64...")
	build := exec.Command("go", "build", "-ldflags=-X main.version="+version, "-o", targetBinary, ".")
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	if out, err := build.CombinedOutput(); err != nil {
		fatalf("build: %v\n%s", err, out)
	}
	fmt.Println(" done")

	// Step 2: copy binary to server
	fmt.Print("Copying kurto to server...")
	scpArgs := append([]string{"-P", port, "-o", "StrictHostKeyChecking=no"}, targetBinary, sshTarget+":/tmp/kurto")
	if key != "" {
		scpArgs = append([]string{"-i", key}, scpArgs...)
	}
	scp := exec.Command("scp", scpArgs...)
	if out, err := scp.CombinedOutput(); err != nil {
		fatalf("scp: %v\n%s", err, out)
	}
	fmt.Println(" done")

	// Step 3: install on server
	fmt.Print("Installing on server...")
	installCmd := "sudo mv /tmp/kurto /usr/local/bin/kurto && sudo chmod +x /usr/local/bin/kurto"
	sshRun(sshTarget, port, key, installCmd)
	fmt.Println(" done")

	// Step 4: copy manifest files
	manifestFiles := findManifests(manifestDir)
	if len(manifestFiles) > 0 {
		fmt.Print("Copying manifests...")
		remoteDir := "/etc/kurto/manifests"
		sshRun(sshTarget, port, key, "sudo mkdir -p "+remoteDir+" && sudo chown "+user+" "+remoteDir)
		for _, f := range manifestFiles {
			remotePath := remoteDir + "/" + filepath.Base(f)
			scpArgs := append([]string{"-P", port, "-o", "StrictHostKeyChecking=no"}, f, sshTarget+":"+remotePath)
			if key != "" {
				scpArgs = append([]string{"-i", key}, scpArgs...)
			}
			exec.Command("scp", scpArgs...).Run()
		}
		fmt.Println(" done")

		// Step 5: apply manifests
		fmt.Print("Applying manifests...")
		applyCmd := "kurto apply -f /etc/kurto/manifests/*.yaml"
		sshRun(sshTarget, port, key, applyCmd)
		fmt.Println(" done")
	}

	// Step 6: status
	fmt.Print("Checking status...")
	statusCmd := "kurto ps -a"
	out, _ := sshOutput(sshTarget, port, key, statusCmd)
	fmt.Println("\n" + out)
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
	args := []string{"-p", port, "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", target, cmd}
	if key != "" {
		args = append([]string{"-i", key}, args...)
	}
	exec.Command("ssh", args...).Run()
}

func sshOutput(target, port, key, cmd string) (string, error) {
	args := []string{"-p", port, "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", target, cmd}
	if key != "" {
		args = append([]string{"-i", key}, args...)
	}
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
