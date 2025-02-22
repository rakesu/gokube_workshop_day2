package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"gokube/pkg/api"
	"gokube/pkg/registry"
	"gokube/pkg/registry/names"
)

// ReplicaSetController manages the lifecycle of ReplicaSets
type ReplicaSetController struct {
	replicaSetRegistry *registry.ReplicaSetRegistry
	podRegistry        *registry.PodRegistry
}

// NewReplicaSetController creates a new ReplicaSetController
func NewReplicaSetController(rsRegistry *registry.ReplicaSetRegistry, podRegistry *registry.PodRegistry) *ReplicaSetController {
	return &ReplicaSetController{
		replicaSetRegistry: rsRegistry,
		podRegistry:        podRegistry,
	}
}

func (rsc *ReplicaSetController) Reconcile(ctx context.Context, rs *api.ReplicaSet) error {
	// Get current ReplicaSet state
	currentRS, err := rsc.replicaSetRegistry.Get(ctx, rs.Name)
	if err != nil {
		return err
	}

	// Get all pods
	allPods, err := rsc.podRegistry.ListPods(ctx)
	if err != nil {
		return err
	}

	// Get active pods for this ReplicaSet
	activePods, err := rsc.getPodsForReplicaSet(currentRS, allPods, api.IsPodActiveAndOwnedBy)
	if err != nil {
		return err
	}

	// Compare current pod count with desired replica count
	currentPodCount := len(activePods)
	desiredPodCount := int(currentRS.Spec.Replicas)

	// Create pods if we have fewer than desired
	for i := currentPodCount; i < desiredPodCount; i++ {
		podName := generatePodNameFromReplicaSet(currentRS.Name)

		newPod := &api.Pod{
			ObjectMeta: api.ObjectMeta{
				Name: podName,
			},
			Spec:   currentRS.Spec.Template.Spec,
			Status: api.PodStatus(api.PodPending),
		}

		if err := rsc.podRegistry.CreatePod(ctx, newPod); err != nil {
			return fmt.Errorf("failed to create pod: %v", err)
		}
	}
	
	currentRS.Status.Replicas = currentRS.Spec.Replicas
    if err := rsc.replicaSetRegistry.Update(ctx, currentRS); err != nil {
        return fmt.Errorf("failed to update ReplicaSet status: %v", err)
    }
	return nil
}

func (rsc *ReplicaSetController) getPodsForReplicaSet(
	rs *api.ReplicaSet,
	allPods []*api.Pod,
	condition func(*api.Pod, *api.ObjectMeta) bool,
) ([]*api.Pod, error) {
	var activePods []*api.Pod
	for _, pod := range allPods {
		if condition(pod, &rs.ObjectMeta) {
			activePods = append(activePods, pod)
		}
	}

	return activePods, nil
}

func (rsc *ReplicaSetController) getPodsOwnedBy(rs *api.ReplicaSet, pods []*api.Pod) ([]*api.Pod, error) {
	return rsc.getPodsForReplicaSet(rs, pods, api.IsOwnedBy)
}

func (rsc *ReplicaSetController) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := rsc.Run(ctx); err != nil {
				fmt.Printf("Error reconciling replicaset: %v\n", err)
			}
		}
	}
}

func (rsc *ReplicaSetController) Run(_ context.Context) error {

	rscList, err := rsc.replicaSetRegistry.List(context.Background())
	if err != nil {
		log.Fatalf("failed to list replicaSets: %v", err)
		return err
	}

	for _, rs := range rscList {
		err := rsc.Reconcile(context.Background(), rs)
		if err != nil {
			log.Fatalf("failed to reconcile: %v", err)
		}
	}
	return nil
}

// GeneratePodNameFromReplicaSet creates a pod name based on the ReplicaSet and container names
func generatePodNameFromReplicaSet(replicaSetName string) string {
	return names.SimpleNameGenerator.GenerateName(replicaSetName)
}
