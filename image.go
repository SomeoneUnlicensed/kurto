package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const dockerHub = "https://registry-1.docker.io"
const authService = "https://auth.docker.io/token"

type tokenResp struct {
	Token string `json:"token"`
}

type manifestV2 struct {
	Layers []struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"layers"`
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
}

func PullImage(nameTag string) error {
	name, tag := nameTag, DefaultTag
	if i := strings.LastIndexByte(nameTag, ':'); i >= 0 {
		name = nameTag[:i]
		tag = nameTag[i+1:]
	}

	store := NewStore()
	if _, err := store.LoadImage(name, tag); err == nil {
		return fmt.Errorf("image %s:%s already exists", name, tag)
	}

	fmt.Printf("Pulling %s:%s...\n", name, tag)

	layerDir := filepath.Join(ImagesDir, name+"@"+tag)
	os.RemoveAll(layerDir)
	os.MkdirAll(layerDir, 0755)

	// Normalize image name for Docker Hub
	imgName := name
	if !strings.Contains(name, "/") {
		imgName = "library/" + name
	}

	digest, layers, totalSize, err := fetchDockerLayers(imgName, tag, layerDir)
	if err != nil || len(layers) == 0 {
		if err != nil {
			fmt.Printf("  Registry: %v\n", err)
		}
		fmt.Println("  Building minimal base image locally")
		rootfsFile := filepath.Join(layerDir, "rootfs.tar.gz")
		if err := buildBaseImage(rootfsFile); err != nil {
			os.RemoveAll(layerDir)
			return err
		}
		fi, _ := os.Stat(rootfsFile)
		if fi != nil {
			totalSize = fi.Size()
		}
		layers = []string{rootfsFile}
		digest = "local"
	}

	img := &Image{
		Name:    name,
		Tag:     tag,
		Digest:  digest,
		Size:    totalSize,
		Created: time.Now(),
		Layers:  layers,
	}
	if err := store.SaveImage(img); err != nil {
		return err
	}

	fmt.Printf("Pulled %s:%s", name, tag)
	if totalSize > 0 {
		fmt.Printf(" (%.1f MB)", float64(totalSize)/1e6)
	}
	fmt.Println()
	return nil
}

func fetchDockerLayers(image, tag, dest string) (digest string, layers []string, totalSize int64, err error) {
	// Get auth token
	token, err := getToken(image)
	if err != nil {
		return "", nil, 0, fmt.Errorf("auth: %w", err)
	}

	// Get manifest
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", dockerHub, image, tag)
	req, _ := http.NewRequest("GET", manifestURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, 0, fmt.Errorf("manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, 0, fmt.Errorf("manifest status: %d", resp.StatusCode)
	}

	var manifest manifestV2
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", nil, 0, fmt.Errorf("manifest decode: %w", err)
	}

	// Download layers
	for i, layer := range manifest.Layers {
		fmt.Printf("  Layer %d/%d: %s\n", i+1, len(manifest.Layers), shortDigest(layer.Digest))

		layerPath := filepath.Join(dest, fmt.Sprintf("layer-%d.tar.gz", i))
		if err := downloadBlob(image, token, layer.Digest, layerPath); err != nil {
			return "", nil, 0, fmt.Errorf("layer %d: %w", i, err)
		}

		layers = append(layers, layerPath)
		totalSize += layer.Size
	}

	return manifest.Config.Digest, layers, totalSize, nil
}

func getToken(image string) (string, error) {
	url := fmt.Sprintf("%s?service=registry.docker.io&scope=repository:%s:pull", authService, image)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var t tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	return t.Token, nil
}

func downloadBlob(image, token, digest, dest string) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", dockerHub, image, digest)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func shortDigest(d string) string {
	if len(d) > 19 {
		return d[:19] + "..."
	}
	return d
}

func buildBaseImage(rootfs string) error {
	f, err := os.Create(rootfs)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	dirs := []string{"bin", "lib", "usr", "etc", "dev", "proc", "sys", "tmp", "home", "root", "mnt", "var"}
	for _, d := range dirs {
		tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: d + "/", Mode: 0755})
	}
	writeFile(tw, "etc/hostname", []byte("kurto\n"), 0644)
	writeFile(tw, "etc/resolv.conf", []byte("nameserver 8.8.8.8\nnameserver 1.1.1.1\n"), 0644)
	writeFile(tw, "etc/hosts", []byte("127.0.0.1 localhost\n::1 localhost\n"), 0644)
	writeFile(tw, "etc/os-release", []byte("NAME=\"Kurto Linux\"\nVERSION_ID=1.0\n"), 0644)
	return nil
}

func writeFile(tw *tar.Writer, name string, data []byte, mode int64) {
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Size: int64(len(data)), Mode: mode})
	tw.Write(data)
}

func extractImage(img *Image, dest string) error {
	os.MkdirAll(dest, 0755)

	if len(img.Layers) == 0 {
		return fmt.Errorf("no layers in image %s:%s", img.Name, img.Tag)
	}

	for _, layer := range img.Layers {
		if err := extractTarGz(layer, dest); err != nil {
			return fmt.Errorf("extract %s: %w", layer, err)
		}
	}
	return nil
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, os.FileMode(hdr.Mode))
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			w, err := os.Create(target)
			if err == nil {
				io.Copy(w, tr)
				w.Chmod(os.FileMode(hdr.Mode))
				w.Close()
			}
		case tar.TypeSymlink:
			os.Remove(target)
			os.Symlink(hdr.Linkname, target)
		case tar.TypeLink:
			os.Remove(target)
			os.Link(filepath.Join(dest, hdr.Linkname), target)
		}
	}
	return nil
}

func removeImageLayers(img *Image) {
	for _, l := range img.Layers {
		os.Remove(l)
	}
}

func ListImages() error {
	images, err := NewStore().ListImages()
	if err != nil {
		return err
	}
	if len(images) == 0 {
		fmt.Println("No images")
		return nil
	}
	fmt.Printf("%-25s %-10s %-10s %-8s %-20s\n", "NAME", "TAG", "SIZE", "OS", "CREATED")
	for _, img := range images {
		sz := ""
		if img.Size > 0 {
			sz = fmt.Sprintf("%.1fMB", float64(img.Size)/1e6)
		} else {
			sz = "built-in"
		}
		osArch := img.OS
		if osArch == "" {
			osArch = "any"
		}
		fmt.Printf("%-25s %-10s %-10s %-8s %-20s\n", img.Name, img.Tag, sz, osArch, img.Created.Format(time.DateTime))
	}
	return nil
}

func RemoveImage(name, tag string) error {
	store := NewStore()
	img, err := store.LoadImage(name, tag)
	if err != nil {
		return fmt.Errorf("image %s:%s not found", name, tag)
	}
	removeImageLayers(img)
	return store.RemoveImage(name, tag)
}
