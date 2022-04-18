package scheduler

import (
	"fmt"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
)

const (
	domain = "sharedgpu/"
	// the name of a pod group that defines a coscheduling pod group.
	PodGroupName = domain + "group_name"
	// the minimum number of pods to be scheduled together in a pod group.
	PodGroupMinAvailable = domain + "min_available"

	PodPriority = domain + "priority"
)

type PodGroupInfo struct {
	// a unique PodGroup ID
	// currently implemented as <namespace>/<PodGroup name>.
	key string
	// the PodGroup name and defined by a pod label
	// the PodGroup name of a regular pod is empty.
	name string
	// the priority is the priority of pods in a podGroup
	// all pods in the same PodGroup should have same priority.
	priority int32
	// stores the initialization timestamp of a PodGroup.
	timestamp time.Time
	// the minimum number of pods to be co-scheduled in a PodGroup
	// all pods in the same PodGroup should have same minAvailable
	minAvailable int
	// stores the timestamp when the PodGroup marked as expired.
	deletionTimestamp *time.Time
}

// returns the existing PodGroup in PodGroupInfos if present.
// Otherwise, it creates a PodGroup and returns the value,
// if the pod defines a PodGroup and its PodGroupMinAvailable is greater than one,
// => it stores the created PodGroup in PodGroupInfo
// => it also returns the pod's PodGroupMinAvailable (0 if not specified).
func (kss *KubeShareScheduler) getOrCreatePodGroupInfo(pod *v1.Pod, ts time.Time) *PodGroupInfo {
	podGroupName, podMinAvailable := kss.getPodGroupLabels(pod)

	var pgKey string
	if len(PodGroupName) > 0 && podMinAvailable > 0 {
		pgKey = fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	}

	kss.podGroupMutex.Lock()
	defer kss.podGroupMutex.Unlock()
	// If it is a PodGroup and present in PodGroupInfos, return it.
	if len(pgKey) != 0 {

		pgInfo, exist := kss.podGroupInfos[pgKey]
		if exist {
			// If the deleteTimestamp isn't nil,
			// it means that the PodGroup is marked as expired before.
			// So we need to set the deleteTimestamp as nil again to mark the PodGroup active.
			if pgInfo.deletionTimestamp != nil {
				pgInfo.deletionTimestamp = nil
				kss.podGroupInfos[pgKey] = pgInfo
			}
			return pgInfo
		}
	}

	// If the PodGroup is not present in PodGroupInfos or the pod is a regular pod,
	// create a PodGroup for the Pod and store it in PodGroupInfos.
	pgInfo := &PodGroupInfo{
		key:          pgKey,
		name:         podGroupName,
		priority:     podutil.GetPodPriority(pod) + kss.getPodPrioriy(pod),
		timestamp:    ts,
		minAvailable: podMinAvailable,
	}
	// If it's not a regular Pod, store the PodGroup in PodGroupInfos
	if len(pgKey) > 0 {
		kss.podGroupInfos[pgKey] = pgInfo
	}
	return pgInfo
}

// checks if the pod belongs to a PodGroup.
// If so,  it will return the podGroupName, minAvailable and priority of the PodGroup.
// If not, it will return "" and 0.

func (kss *KubeShareScheduler) getPodGroupLabels(pod *v1.Pod) (string, int) {
	podGroupName, ok := pod.Labels[PodGroupName]
	if !ok || len(podGroupName) == 0 {
		return "", 0
	}
	minAvailable, ok := pod.Labels[PodGroupMinAvailable]
	if !ok || len(minAvailable) == 0 {
		return "", 0
	}

	miniNum, err := strconv.Atoi(minAvailable)
	if err != nil || miniNum < 1 {
		kss.ksl.Error(fmt.Sprintf("PodGroup %v/%v : PodGroupMinAvailable %v is invalid", pod.Namespace, pod.Name, minAvailable))
		return "", 0
	}

	return podGroupName, miniNum
}

// Be careful, the default priority is 1
func (kss *KubeShareScheduler) getPodPrioriy(pod *v1.Pod) int32 {

	priority, ok := pod.Labels[PodPriority]
	if !ok || len(priority) == 0 {
		return 1
	}

	p, err := strconv.Atoi(priority)
	if err != nil || p < 1 {
		kss.ksl.Error(fmt.Sprintf("Pod %v/%v : Priority %v is invalid", pod.Namespace, pod.Name, priority))
	}

	return int32(p)
}
