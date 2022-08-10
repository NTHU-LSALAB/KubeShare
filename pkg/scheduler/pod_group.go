package scheduler

import (
	"fmt"
	"math"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
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
	// the minimum number of pod = headCount*threshold
	minAvailable int
        // the total number of pods in a PodGroup
	headCount int
        // the minimum proportion of pods to be scheduled together in a PodGroup
	threshold float64

	// stores the timestamp when the PodGroup marked as expired.
	deletionTimestamp *time.Time
}

// returns the existing PodGroup in PodGroupInfos if present.
// Otherwise, it creates a PodGroup and returns the value,
// if the pod defines a PodGroup and its PodGroupMinAvailable is greater than one,
// => it stores the created PodGroup in PodGroupInfo
// => it also returns the pod's PodGroupMinAvailable (0 if not specified).
func (kss *KubeShareScheduler) getOrCreatePodGroupInfo(pod *v1.Pod, ts time.Time) *PodGroupInfo {
	podGroupName, podGroupHeadcount, podGroupThreshold, podMinAvailable := kss.getPodGroupLabels(pod)

	var pgKey string
	if len(PodGroupName) > 0 && podMinAvailable > 0 {
		pgKey = fmt.Sprintf("%v/%v", pod.Namespace, podGroupName)
	}

	kss.podGroupMutex.Lock()
	defer kss.podGroupMutex.Unlock()
	// If it is a PodGroup and present in PodGroupInfos, return it.
	if pgKey != "" {
		if pgInfo, ok := kss.podGroupInfos[pgKey]; ok {
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

	_, _, priority := kss.getPodPrioriy(pod)
	// If the PodGroup is not present in PodGroupInfos or the pod is a regular pod,
	// create a PodGroup for the Pod and store it in PodGroupInfos.
	pgInfo := &PodGroupInfo{
		key:          pgKey,
		name:         podGroupName,
		priority:     priority, // podutil.GetPodPriority(pod) + kss.getPodPrioriy(pod)
		timestamp:    ts,
		minAvailable: podMinAvailable,
		headCount:    podGroupHeadcount,
		threshold:    podGroupThreshold,
	}
	// If it's not a regular Pod, store the PodGroup in PodGroupInfos
	if pgKey != "" {
		kss.podGroupInfos[pgKey] = pgInfo
	}
	return pgInfo
}

// checks if the pod belongs to a PodGroup.
// If so,  it will return the podGroupName, headcount, threshold, minAvailable.
// If not, it will return "" and 0.
func (kss *KubeShareScheduler) getPodGroupLabels(pod *v1.Pod) (string, int, float64, int) {
	podGroupName, ok := pod.Labels[PodGroupName]
	if !ok || len(podGroupName) == 0 {
		return "", 0, 0.0, 0
	}

	headcount, ok := pod.Labels[PodGroupHeadcount]
	if !ok || len(headcount) == 0 {
		return "", 0, 0.0, 0
	}

	headcnt, err := strconv.Atoi(headcount)
	if err != nil || headcnt < 1 {
		kss.ksl.Error(fmt.Sprintf("PodGroup %v/%v : PodGroupHeadCount %v is invalid", pod.Namespace, pod.Name, headcount))
		return "", 0, 0.0, 0
	}

	threshold, ok := pod.Labels[PodGroupThreshold]
	if !ok || len(threshold) == 0 {
		return "", 0, 0.0, 0
	}

	thres, err := strconv.ParseFloat(threshold, 64)
	if err != nil || thres <= 0 {
		kss.ksl.Error(fmt.Sprintf("PodGroup %v/%v : PodGroupThreshold %v is invalid", pod.Namespace, pod.Name, threshold))
		return "", 0, 0.0, 0
	}

	minAvailable := int(math.Floor(thres*float64(headcnt) + 0.5))

	return podGroupName, headcnt, thres, minAvailable
}

func (kss *KubeShareScheduler) podGroupInfoGC() {
	kss.podGroupMutex.Lock()
	defer kss.podGroupMutex.Unlock()

	for key, pgInfo := range kss.podGroupInfos {
		if pgInfo.deletionTimestamp != nil && pgInfo.deletionTimestamp.Add(time.Duration(kss.args.PodGroupExpirationTimeSeconds)*time.Second).Before(kss.clock.Now()) {
			kss.ksl.Warn(key, " is out of date and has been deleted in PodGroup GarbegeCollection")
			delete(kss.podGroupInfos, key)
		}
	}
}
