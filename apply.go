package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func ApplyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	// Split YAML documents (multi-doc support)
	docs := splitYAML(string(data))
	if len(docs) == 0 {
		return fmt.Errorf("no YAML documents in %s", path)
	}

	for i, doc := range docs {
		var res Resource
		if err := yaml.Unmarshal([]byte(doc), &res); err != nil {
			return fmt.Errorf("document %d: %w", i+1, err)
		}

		switch res.Kind {
		case "Pod":
			if err := applyPod([]byte(doc)); err != nil {
				return fmt.Errorf("document %d (Pod): %w", i+1, err)
			}
		case "Container":
			if err := applyContainer([]byte(doc)); err != nil {
				return fmt.Errorf("document %d (Container): %w", i+1, err)
			}
		default:
			return fmt.Errorf("document %d: unknown kind %q", i+1, res.Kind)
		}
	}
	return nil
}

func applyPod(data []byte) error {
	var pr PodResource
	if err := yaml.Unmarshal(data, &pr); err != nil {
		return err
	}

	spec := pr.Spec
	spec.Name = pr.Metadata.Name
	spec.Namespace = pr.Metadata.Namespace
	if spec.Namespace == "" {
		spec.Namespace = DefaultNS
	}
	spec.Labels = pr.Metadata.Labels

	_, err := CreatePod(spec)
	return err
}

func applyContainer(data []byte) error {
	var cr struct {
		APIVersion string          `yaml:"apiVersion"`
		Kind       string          `yaml:"kind"`
		Metadata   ResourceMeta    `yaml:"metadata"`
		Spec       ContainerConfig `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &cr); err != nil {
		return err
	}

	cfg := cr.Spec
	cfg.Name = cr.Metadata.Name
	cfg.Namespace = cr.Metadata.Namespace
	if cfg.Namespace == "" {
		cfg.Namespace = DefaultNS
	}
	if cfg.Labels == nil {
		cfg.Labels = cr.Metadata.Labels
	} else {
		for k, v := range cr.Metadata.Labels {
			cfg.Labels[k] = v
		}
	}

	c, err := CreateContainer(cfg)
	if err != nil {
		return err
	}
	return StartContainer(c.ID)
}

func GetResources(kind, namespace string, labelSelector map[string]string) error {
	switch kind {
	case "pods", "pod", "po":
		return ListPods(namespace, labelSelector)
	case "containers", "container", "ct", "c":
		return ListContainers(namespace, true, labelSelector)
	case "images", "image", "im":
		return ListImages()
	default:
		return fmt.Errorf("unknown resource type: %s", kind)
	}
}

func DeleteResource(kind, name string) error {
	switch kind {
	case "pods", "pod", "po":
		pod, err := NewStore().FindPod(name)
		if err != nil {
			return err
		}
		return RemovePod(pod.ID)
	case "containers", "container", "ct", "c":
		ct, err := NewStore().FindContainer(name)
		if err != nil {
			return err
		}
		return RemoveContainer(ct.ID)
	case "images", "image", "im":
		imgName, imgTag := name, DefaultTag
		if i := strings.LastIndexByte(name, ':'); i >= 0 {
			imgName = name[:i]
			imgTag = name[i+1:]
		}
		return RemoveImage(imgName, imgTag)
	default:
		return fmt.Errorf("unknown resource type: %s", kind)
	}
}

func DescribeResource(kind, name string) error {
	switch kind {
	case "pods", "pod", "po":
		pod, err := NewStore().FindPod(name)
		if err != nil {
			return err
		}
		fmt.Printf("Pod:     %s\n", pod.Name)
		fmt.Printf("ID:      %s\n", pod.ID)
		fmt.Printf("Status:  %s\n", pod.Status)
		fmt.Printf("Created: %s\n", pod.Created.Format("2006-01-02 15:04:05"))
		fmt.Printf("Containers:\n")
		for _, cid := range pod.ContainerIDs {
			ct, err := NewStore().LoadContainer(cid)
			if err != nil {
				continue
			}
			fmt.Printf("  - %s (%s) [%s]\n", ct.Name, ct.Image, ct.Status)
		}
		return nil
	case "containers", "container", "ct", "c":
		ct, err := NewStore().FindContainer(name)
		if err != nil {
			return err
		}
		fmt.Printf("Container: %s\n", ct.Name)
		fmt.Printf("ID:        %s\n", ct.ID)
		fmt.Printf("Image:     %s\n", ct.Image)
		fmt.Printf("Status:    %s\n", ct.Status)
		fmt.Printf("PID:       %d\n", ct.PID)
		fmt.Printf("Platform:  %s\n", ct.PlatformOS)
		fmt.Printf("Created:   %s\n", ct.Created.Format("2006-01-02 15:04:05"))
		return nil
	default:
		return fmt.Errorf("unknown resource type: %s", kind)
	}
}

func splitYAML(s string) []string {
	var docs []string
	var current strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "---" {
			if current.Len() > 0 {
				docs = append(docs, current.String())
			}
			current.Reset()
		} else {
			current.WriteString(line)
			current.WriteByte('\n')
		}
	}
	if current.Len() > 0 {
		docs = append(docs, current.String())
	}
	return docs
}
