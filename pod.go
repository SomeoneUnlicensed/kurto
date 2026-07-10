package main

import (
	"fmt"
	"time"
)

func CreatePod(spec PodSpec) (*Pod, error) {
	id := NewID()
	if spec.Name == "" {
		spec.Name = id[:12]
	}
	if spec.Namespace == "" {
		spec.Namespace = DefaultNS
	}
	if spec.RestartPolicy == "" {
		spec.RestartPolicy = "never"
	}

	pod := &Pod{
		ID:        id,
		Name:      spec.Name,
		Namespace: spec.Namespace,
		Labels:    spec.Labels,
		Status:    PodPending,
		Created:   time.Now(),
		Spec:      spec,
	}

	if err := NewStore().SavePod(pod); err != nil {
		return nil, err
	}

	// Start all containers in the pod
	for _, cc := range spec.Containers {
		cc.PodID = id
		cc.Labels = mergeLabels(cc.Labels, spec.Labels)
		cc.Namespace = spec.Namespace
		if cc.Name == "" {
			cc.Name = cc.Image
		}

		c, err := CreateContainer(cc)
		if err != nil {
			pod.Status = PodFailed
			NewStore().SavePod(pod)
			return nil, fmt.Errorf("pod %s: create container %s: %w", spec.Name, cc.Name, err)
		}
		pod.ContainerIDs = append(pod.ContainerIDs, c.ID)

		if err := StartContainer(c.ID); err != nil {
			pod.Status = PodFailed
			NewStore().SavePod(pod)
			return nil, fmt.Errorf("pod %s: start container %s: %w", spec.Name, cc.Name, err)
		}
	}

	pod.Status = PodRunning
	NewStore().SavePod(pod)

	fmt.Printf("Pod %s/%s created (%d containers)\n", spec.Namespace, spec.Name, len(spec.Containers))
	return pod, nil
}

func StopPod(id string) error {
	store := NewStore()
	pod, err := store.FindPod(id)
	if err != nil {
		return err
	}
	if pod.Status == PodSucceeded || pod.Status == PodFailed {
		return fmt.Errorf("pod %s already finished", pod.Name)
	}
	for _, cid := range pod.ContainerIDs {
		StopContainer(cid)
	}
	pod.Status = PodSucceeded
	return store.SavePod(pod)
}

func RemovePod(id string) error {
	store := NewStore()
	pod, err := store.FindPod(id)
	if err != nil {
		return err
	}
	for _, cid := range pod.ContainerIDs {
		store.RemoveContainer(cid)
	}
	return store.RemovePod(pod.ID)
}

func ListPods(namespace string, labelSelector map[string]string) error {
	store := NewStore()
	pods, err := store.ListPods(namespace)
	if err != nil {
		return err
	}
	if len(pods) == 0 {
		fmt.Println("No pods")
		return nil
	}
	fmt.Printf("%-12s %-25s %-12s %-10s %-8s %-20s\n", "ID", "NAME", "NAMESPACE", "STATUS", "#CT", "CREATED")
	for _, p := range pods {
		if labelSelector != nil && !matchLabels(p.Labels, labelSelector) {
			continue
		}
		fmt.Printf("%-12s %-25s %-12s %-10s %-8d %-20s\n",
			p.ID[:12], p.Name, p.Namespace, p.Status, len(p.ContainerIDs),
			p.Created.Format(time.Stamp))
	}
	return nil
}

func GetPodStatus(id string) (*Pod, error) {
	store := NewStore()
	return store.FindPod(id)
}

func mergeLabels(a, b map[string]string) map[string]string {
	if a == nil {
		a = make(map[string]string)
	}
	for k, v := range b {
		if _, ok := a[k]; !ok {
			a[k] = v
		}
	}
	return a
}
