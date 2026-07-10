package main

import "time"

const (
	RootDir       = "/var/lib/kurto"
	ContainersDir = RootDir + "/containers"
	ImagesDir     = RootDir + "/images"
	PodsDir       = RootDir + "/pods"
	DefaultTag    = "latest"
	DefaultNS     = "default"
)

type ContainerStatus string

const (
	StatusCreated ContainerStatus = "created"
	StatusRunning ContainerStatus = "running"
	StatusStopped ContainerStatus = "stopped"
	StatusExited  ContainerStatus = "exited"
)

type PortMapping struct {
	HostPort      int    `json:"host_port" yaml:"hostPort"`
	ContainerPort int    `json:"container_port" yaml:"containerPort"`
	Protocol      string `json:"protocol" yaml:"protocol"`
}

type Mount struct {
	Source string `json:"source" yaml:"source"`
	Target string `json:"target" yaml:"target"`
	Type   string `json:"type" yaml:"type"`
}

type EnvVar struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

type ResourceSpec struct {
	Memory string `json:"memory" yaml:"memory"`
	CPUs   string `json:"cpus" yaml:"cpus"`
}

type ContainerConfig struct {
	Name       string         `json:"name" yaml:"name"`
	Image      string         `json:"image" yaml:"image"`
	Command    []string       `json:"command,omitempty" yaml:"command,omitempty"`
	Args       []string       `json:"args,omitempty" yaml:"args,omitempty"`
	Env        []EnvVar       `json:"env,omitempty" yaml:"env,omitempty"`
	Ports      []PortMapping  `json:"ports,omitempty" yaml:"ports,omitempty"`
	Mounts     []Mount        `json:"mounts,omitempty" yaml:"mounts,omitempty"`
	Resources  ResourceSpec   `json:"resources,omitempty" yaml:"resources,omitempty"`
	WorkingDir string         `json:"working_dir,omitempty" yaml:"workingDir,omitempty"`
	Labels     map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	AutoRemove bool           `json:"auto_remove" yaml:"autoRemove"`
	PodID      string         `json:"pod_id,omitempty" yaml:"-"`
	Namespace  string         `json:"namespace" yaml:"namespace"`
}

type Container struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	PID        int               `json:"pid"`
	Status     ContainerStatus   `json:"status"`
	Created    time.Time         `json:"created"`
	ExitCode   int               `json:"exit_code"`
	Config     ContainerConfig   `json:"config"`
	PlatformOS string            `json:"platform_os"`
}

type Image struct {
	Name       string    `json:"name"`
	Tag        string    `json:"tag"`
	Digest     string    `json:"digest"`
	Size       int64     `json:"size"`
	Created    time.Time `json:"created"`
	Layers     []string  `json:"layers"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
}

type PodStatus string

const (
	PodPending   PodStatus = "pending"
	PodRunning   PodStatus = "running"
	PodSucceeded PodStatus = "succeeded"
	PodFailed    PodStatus = "failed"
)

type PodSpec struct {
	Name        string            `json:"name" yaml:"name"`
	Namespace   string            `json:"namespace" yaml:"namespace"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Containers  []ContainerConfig `json:"containers" yaml:"containers"`
	RestartPolicy string          `json:"restart_policy" yaml:"restartPolicy"`
}

type Pod struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
	Status    PodStatus         `json:"status"`
	Created   time.Time         `json:"created"`
	Spec      PodSpec           `json:"spec"`
	ContainerIDs []string       `json:"container_ids"`
}

type Resource struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   ResourceMeta `yaml:"metadata"`
	Spec       interface{} `yaml:"spec"`
}

type ResourceMeta struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

type PodResource struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   ResourceMeta    `yaml:"metadata"`
	Spec       PodSpec         `yaml:"spec"`
}
