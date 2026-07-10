package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	mu sync.RWMutex
}

func NewStore() *Store {
	for _, d := range []string{RootDir, ContainersDir, ImagesDir, PodsDir} {
		os.MkdirAll(d, 0755)
	}
	return &Store{}
}

func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x%x%x%x", b[0:2], b[2:4], b[4:6], b[6:8])
}

// ─── Containers ───────────────────────────────────────────

func (s *Store) SaveContainer(c *Container) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := filepath.Join(ContainersDir, c.ID, "config.json")
	os.MkdirAll(filepath.Dir(p), 0755)
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(c)
}

func (s *Store) LoadContainer(id string) (*Container, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(filepath.Join(ContainersDir, id, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c Container
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListContainers(namespace string) ([]*Container, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(ContainersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var list []*Container
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c, err := s.LoadContainer(e.Name())
		if err != nil {
			continue
		}
		if namespace == "" || c.Config.Namespace == namespace {
			list = append(list, c)
		}
	}
	return list, nil
}

func (s *Store) RemoveContainer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.RemoveAll(filepath.Join(ContainersDir, id))
}

func (s *Store) FindContainer(ref string) (*Container, error) {
	all, err := s.ListContainers("")
	if err != nil {
		return nil, err
	}
	var match *Container
	for _, c := range all {
		if c.ID == ref || c.Name == ref {
			return c, nil
		}
	}
	for _, c := range all {
		if len(ref) >= 3 && (startsWith(c.ID, ref) || startsWith(c.Name, ref)) {
			if match != nil {
				return nil, fmt.Errorf("multiple containers match %q", ref)
			}
			match = c
		}
	}
	if match == nil {
		return nil, fmt.Errorf("container %q not found", ref)
	}
	return match, nil
}

// ─── Images ───────────────────────────────────────────────

func (s *Store) SaveImage(img *Image) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := filepath.Join(ImagesDir, img.Name+"@"+img.Tag+".json")
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(img)
}

func (s *Store) LoadImage(name, tag string) (*Image, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(filepath.Join(ImagesDir, name+"@"+tag+".json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var img Image
	if err := json.NewDecoder(f).Decode(&img); err != nil {
		return nil, err
	}
	return &img, nil
}

func (s *Store) ListImages() ([]*Image, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(ImagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var list []*Image
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		f, err := os.Open(filepath.Join(ImagesDir, e.Name()))
		if err != nil {
			continue
		}
		var img Image
		if json.NewDecoder(f).Decode(&img) == nil {
			list = append(list, &img)
		}
		f.Close()
	}
	return list, nil
}

func (s *Store) RemoveImage(name, tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := filepath.Join(ImagesDir, name+"@"+tag+".json")
	os.Remove(p)
	return os.RemoveAll(filepath.Join(ImagesDir, name+"@"+tag))
}

// ─── Pods ─────────────────────────────────────────────────

func (s *Store) SavePod(p *Pod) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	podDir := filepath.Join(PodsDir, p.ID)
	os.MkdirAll(podDir, 0755)
	f, err := os.Create(filepath.Join(podDir, "pod.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(p)
}

func (s *Store) LoadPod(id string) (*Pod, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(filepath.Join(PodsDir, id, "pod.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var p Pod
	if err := json.NewDecoder(f).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListPods(namespace string) ([]*Pod, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(PodsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var list []*Pod
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := s.LoadPod(e.Name())
		if err != nil {
			continue
		}
		if namespace == "" || p.Namespace == namespace {
			list = append(list, p)
		}
	}
	return list, nil
}

func (s *Store) RemovePod(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.RemoveAll(filepath.Join(PodsDir, id))
}

func (s *Store) FindPod(ref string) (*Pod, error) {
	all, err := s.ListPods("")
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		if p.ID == ref || p.Name == ref {
			return p, nil
		}
	}
	for _, p := range all {
		if len(ref) >= 3 && (startsWith(p.ID, ref) || startsWith(p.Name, ref)) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("pod %q not found", ref)
}

// ─── Helpers ──────────────────────────────────────────────

func startsWith(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func matchLabels(obj map[string]string, selector map[string]string) bool {
	for k, v := range selector {
		if obj[k] != v {
			return false
		}
	}
	return true
}


