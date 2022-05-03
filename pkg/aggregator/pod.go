package aggregator

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// scheduler name
	schedulerName = "kubeshare-scheduler"
)

var (
	domain = "sharedgpu/"

	// the name of a pod group that defines a coscheduling pod group.
	PodGroupName = domain + "group_name"
	// the minimum number of pods to be scheduled together in a pod group.
	PodGroupMinAvailable = domain + "min_available"

	// the upper limit percentage of time over the past sample period that one or more kernels of the pod are executed on the GPU
	KubeShareResourceGPULimit = domain + "gpu_limit"
	// the minimum request percentage of time over the past sample period that one or more kernels of the pod are executed on the GPU
	KubeShareResourceGPURequest = domain + "gpu_request"
	// the gpu memory request (in Byte)
	KubeShareResourceGPUMemory = domain + "gpu_mem"

	// the binding cell id
	KubeShareResourceCellID = domain + "cell_id"
)

type PodInfo struct {
	namespace    string
	name         string
	podId        string
	nodeName     string
	groupName    string
	minAvailable string
	limit        string
	request      string
	memory       string
	cellID       string
	uuid         string
	port         string
}

func (a *Aggregator) getPods() []*PodInfo {
	runningPods, err := a.clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{FieldSelector: "spec.schedulerName=" + schedulerName + ",status.phase=Running"})
	if err != nil {
		a.ksl.Fatalf("Error when listing the pod information: %s", err.Error())
	}

	pods := runningPods.Items

	// n := len(pods)
	//podInfos := make([]*PodInfo, n)

	podInfos := []*PodInfo{}
	for _, pod := range pods {
		p := processPod(&pod)

		if p != nil {
			// podInfos[i] = p
			podInfos = append(podInfos, p)
		}
	}

	return podInfos
}

func processPod(pod *v1.Pod) *PodInfo {

	limit, ok := pod.Labels[KubeShareResourceGPULimit]

	if !ok {
		limit = "0.0"
		return nil
	}

	namespace := pod.ObjectMeta.Namespace
	name := pod.ObjectMeta.Name
	key := fmt.Sprintf("%v/%v", namespace, name)

	groupName, ok := pod.Labels[PodGroupName]
	if !ok {
		groupName = key
	}

	minAvailable, ok := pod.Labels[PodGroupMinAvailable]
	if !ok {
		minAvailable = "1"
	}

	request, ok := pod.Labels[KubeShareResourceGPURequest]
	if !ok {
		request = "0.0"
	}

	memory, ok := pod.Labels[KubeShareResourceGPUMemory]
	if !ok {
		memory, ok = pod.Annotations[KubeShareResourceGPUMemory]
		if !ok {
			memory = "0"
		}
	}

	uuid, port := getGPUUUIDPort(pod)

	cellID := pod.Annotations[KubeShareResourceCellID]

	return &PodInfo{
		namespace:    pod.ObjectMeta.Namespace,
		name:         pod.ObjectMeta.Name,
		podId:        string(pod.ObjectMeta.UID),
		nodeName:     pod.Spec.NodeName,
		groupName:    groupName,
		minAvailable: minAvailable,
		limit:        limit,
		request:      request,
		memory:       memory,
		cellID:       cellID,
		uuid:         uuid,
		port:         port,
	}
}

func getGPUUUIDPort(pod *v1.Pod) (string, string) {
	uuid := ""
	port := "0"
	containers := len(pod.Spec.Containers)
	findUUID := false
	findPort := false

	for i := 0; i < containers; i++ {
		n := len(pod.Spec.Containers[i].Env)
		for j := 0; j < n; j++ {
			if pod.Spec.Containers[i].Env[j].Name == "NVIDIA_VISIBLE_DEVICES" {
				uuid = pod.Spec.Containers[i].Env[j].Value
				findUUID = true

			} else if pod.Spec.Containers[i].Env[j].Name == "POD_MANAGER_PORT" {
				port = pod.Spec.Containers[i].Env[j].Value
				findPort = true
			}
		}
		if findUUID && findPort {
			break
		}
	}
	return uuid, port
}
