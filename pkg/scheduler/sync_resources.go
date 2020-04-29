package scheduler

import (
	"math"
	"strconv"
	"strings"
	"sync"

	kubesharev1 "github.com/NTHU-LSALAB/KubeShare/pkg/apis/kubeshare/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

func syncClusterResources(nodeList []*corev1.Node, podList []*corev1.Pod, sharePodList []*kubesharev1.SharePod) (nodeResources NodeResources) {
	nodeResources = syncNodeResources(nodeList)
	syncPodResources(nodeResources, podList, sharePodList)
	return
}

func syncPodResources(nodeRes NodeResources, podList []*corev1.Pod, sharePodList []*kubesharev1.SharePod) {
	for _, pod := range podList {
		nodeName := pod.Spec.NodeName
		// 1. If Pod is not scheduled, it don't use resources.
		// 2. If Pod's name contains "sharepod-dummypod" is managed by SharePod,
		//    resource usage will be calcuated later.
		if nodeName == "" || strings.Contains(pod.Name, kubesharev1.KubeShareDummyPodName) {
			continue
		}
		// If a Pod is owned by a SharePod, calculating their resource usage later.
		ownedBySharePod := false
		for _, owneref := range pod.ObjectMeta.OwnerReferences {
			if owneref.Kind == "SharePod" {
				ownedBySharePod = true
				break
			}
		}
		if ownedBySharePod {
			continue
		}
		// If a running Pod is on the node we don't want, don't calculate it.
		// ex. on master has NoSchedule Taint.
		if _, ok := nodeRes[nodeName]; !ok {
			continue
		}

		if (pod.Spec.RestartPolicy == corev1.RestartPolicyOnFailure &&
			pod.Status.Phase == corev1.PodSucceeded) ||
			(pod.Spec.RestartPolicy == corev1.RestartPolicyNever &&
				(pod.Status.Phase == corev1.PodSucceeded ||
					pod.Status.Phase == corev1.PodFailed)) {
			continue
		}

		for _, container := range pod.Spec.Containers {
			nodeRes[nodeName].CpuFree -= container.Resources.Requests.Cpu().MilliValue()
			nodeRes[nodeName].MemFree -= container.Resources.Requests.Memory().MilliValue()
			gpu := container.Resources.Requests[kubesharev1.ResourceNVIDIAGPU]
			nodeRes[nodeName].GpuFreeCount -= int(gpu.Value())
		}
	}

	for _, sharePod := range sharePodList {
		nodeName := sharePod.Spec.NodeName
		// 1. If Pod is not scheduled, it don't use resources.
		if nodeName == "" {
			continue
		}
		// If a running Pod is on the node we don't want, don't calculate it.
		// ex. on master has NoSchedule Taint.
		if _, ok := nodeRes[nodeName]; !ok {
			continue
		}
		if sharePod.Status.PodStatus != nil {
			// why policy Always is ignored? why??? I forgot why wrote this then
			// if (sharePod.Spec.RestartPolicy == corev1.RestartPolicyAlways) ||
			if (sharePod.Spec.RestartPolicy == corev1.RestartPolicyOnFailure &&
				sharePod.Status.PodStatus.Phase == corev1.PodSucceeded) ||
				(sharePod.Spec.RestartPolicy == corev1.RestartPolicyNever &&
					(sharePod.Status.PodStatus.Phase == corev1.PodSucceeded ||
						sharePod.Status.PodStatus.Phase == corev1.PodFailed)) {
				continue
			}
		}

		for _, container := range sharePod.Spec.Containers {
			nodeRes[nodeName].CpuFree -= container.Resources.Requests.Cpu().MilliValue()
			nodeRes[nodeName].MemFree -= container.Resources.Requests.Memory().MilliValue()
		}

		isGPUPod := false
		gpu_request := 0.0
		gpu_limit := 0.0
		gpu_mem := int64(0)
		GPUID := ""
		affinityTag := ""
		antiAffinityTag := ""
		exclusionTag := ""

		if sharePod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPURequest] != "" ||
			sharePod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPULimit] != "" ||
			sharePod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPUMemory] != "" ||
			sharePod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPUID] != "" {
			var err error
			gpu_limit, err = strconv.ParseFloat(sharePod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPULimit], 64)
			if err != nil || gpu_limit > 1.0 || gpu_limit < 0.0 {
				continue
			}
			gpu_request, err = strconv.ParseFloat(sharePod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPURequest], 64)
			if err != nil || gpu_request > gpu_limit || gpu_request < 0.0 {
				continue
			}
			gpu_mem, err = strconv.ParseInt(sharePod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPUMemory], 10, 64)
			if err != nil || gpu_mem < 0 {
				continue
			}
			GPUID = sharePod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPUID]
			isGPUPod = true
		}

		if val, ok := sharePod.ObjectMeta.Annotations[KubeShareScheduleAffinity]; ok {
			affinityTag = val
		}
		if val, ok := sharePod.ObjectMeta.Annotations[KubeShareScheduleAntiAffinity]; ok {
			antiAffinityTag = val
		}
		if val, ok := sharePod.ObjectMeta.Annotations[KubeShareScheduleExclusion]; ok {
			exclusionTag = val
		}

		if isGPUPod {
			if gpuInfo, ok := nodeRes[nodeName].GpuFree[GPUID]; !ok {
				if nodeRes[nodeName].GpuFreeCount > 0 {
					nodeRes[nodeName].GpuFreeCount--
					nodeRes[nodeName].GpuFree[GPUID] = &GPUInfo{
						GPUFreeReq: 1000 - int64(math.Ceil(gpu_request*(float64)(1000.0))),
						GPUFreeMem: nodeRes[nodeName].GpuMemTotal - gpu_mem,
					}
				} else {
					klog.Errorf("==================================")
					klog.Errorf("Bug! The rest number of free GPU is not enough for SharePod! GPUID: %s", GPUID)
					for errID, errGPU := range nodeRes[nodeName].GpuFree {
						klog.Errorf("GPUID: %s", errID)
						klog.Errorf("    Req: %d", errGPU.GPUFreeReq)
						klog.Errorf("    Mem: %d", errGPU.GPUFreeMem)
					}
					klog.Errorf("==================================")
					continue
				}
			} else {
				gpuInfo.GPUFreeReq -= int64(math.Ceil(gpu_request * (float64)(1000.0)))
				gpuInfo.GPUFreeMem -= gpu_mem
			}

			if affinityTag != "" {
				isFound := false
				for _, val := range nodeRes[nodeName].GpuFree[GPUID].GPUAffinityTags {
					if val == affinityTag {
						isFound = true
						break
					}
				}
				if !isFound {
					nodeRes[nodeName].GpuFree[GPUID].GPUAffinityTags = append(nodeRes[nodeName].GpuFree[GPUID].GPUAffinityTags, affinityTag)
				}
			}
			if antiAffinityTag != "" {
				isFound := false
				for _, val := range nodeRes[nodeName].GpuFree[GPUID].GPUAntiAffinityTags {
					if val == antiAffinityTag {
						isFound = true
						break
					}
				}
				if !isFound {
					nodeRes[nodeName].GpuFree[GPUID].GPUAntiAffinityTags = append(nodeRes[nodeName].GpuFree[GPUID].GPUAntiAffinityTags, antiAffinityTag)
				}
			}
			if exclusionTag != "" {
				isFound := false
				for _, val := range nodeRes[nodeName].GpuFree[GPUID].GPUExclusionTags {
					if val == exclusionTag {
						isFound = true
						break
					}
				}
				if !isFound {
					nodeRes[nodeName].GpuFree[GPUID].GPUExclusionTags = append(nodeRes[nodeName].GpuFree[GPUID].GPUExclusionTags, exclusionTag)
				}
			}
		}
	}
}

func syncNodeResources(nodeList []*corev1.Node) (nodeResources NodeResources) {

	nodeResources = make(NodeResources, len(nodeList))
	var nodeResourcesMux sync.Mutex
	var wait sync.WaitGroup

	syncNode := func(node *corev1.Node) {
		// If NoSchedule Taint on the node, don't add to NodeResources!
		cannotScheduled := false
		for _, taint := range node.Spec.Taints {
			if string(taint.Effect) == "NoSchedule" {
				cannotScheduled = true
				klog.Info("Node have NoSchedule taint, node name: ", node.ObjectMeta.Name)
				break
			}
		}
		if cannotScheduled {
			return
		}

		cpu := node.Status.Allocatable.Cpu().MilliValue()
		mem := node.Status.Allocatable.Memory().MilliValue()
		gpuNum := func() int {
			tmp := node.Status.Allocatable[kubesharev1.ResourceNVIDIAGPU]
			return int(tmp.Value())
		}()
		gpuMem := func() int64 {
			if gpuInfo, ok := node.ObjectMeta.Annotations[kubesharev1.KubeShareNodeGPUInfo]; ok {
				gpuInfoArr := strings.Split(gpuInfo, ",")
				if len(gpuInfoArr) >= 1 {
					gpuArr := strings.Split(gpuInfoArr[0], ":")
					if len(gpuArr) != 2 {
						klog.Errorf("GPU Info format error: %s", gpuInfo)
						return 0
					}
					gpuMem, err := strconv.ParseInt(gpuArr[1], 10, 64)
					if err != nil {
						klog.Errorf("GPU Info format error: %s", gpuInfo)
						return 0
					}
					return gpuMem
				} else {
					return 0
				}
			} else {
				return 0
			}
		}()
		nodeResourcesMux.Lock()
		nodeResources[node.ObjectMeta.Name] = &NodeResource{
			CpuTotal:     cpu,
			MemTotal:     mem,
			GpuTotal:     gpuNum,
			GpuMemTotal:  gpuMem * 1024 * 1024, // in bytes
			CpuFree:      cpu,
			MemFree:      mem,
			GpuFreeCount: gpuNum,
			GpuFree:      make(map[string]*GPUInfo, gpuNum),
		}
		nodeResourcesMux.Unlock()
		wait.Done()
	}

	wait.Add(len(nodeList))
	for _, node := range nodeList {
		go syncNode(node)
	}
	wait.Wait()
	return
}
