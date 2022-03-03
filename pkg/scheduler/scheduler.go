package scheduler

import (
	"math"
	"sync"

	corev1 "k8s.io/api/core/v1"

	sharedgpuv1 "KubeShare/pkg/apis/sharedgpu/v1"
)

func scheduleSharePod(isGPUPod bool, gpu_request float64, gpu_mem int64, gpu_mem_set bool, sharepod *sharedgpuv1.SharePod, nodeList []*corev1.Node, podList []*corev1.Pod, sharePodList []*sharedgpuv1.SharePod) (string, string, int64) {

	// Implement custom scheduling algorithm and replace the function assignment
	// Prototype: FUNC(bool, string, *sharedgpuv1.SharePod, NodeResources) (string, string, error)
	ap := ScheduleAlgorithmBestFit

	nodeResources := syncClusterResources(nodeList, podList, sharePodList)
	for _, filter := range filters {
		filter(nodeResources, nodeList, sharepod)
	}
	return ap(isGPUPod, gpu_request, gpu_mem, gpu_mem_set, sharepod, nodeResources)
}

func ScheduleAlgorithmBestFit(isGPUPod bool, gpu_request float64, gpu_mem int64, gpu_mem_set bool, sharepod *sharedgpuv1.SharePod, nodeResources NodeResources) (schedNodeName string, schedGPUID string, schedPodMem int64) {
	type candidateNodeGPU struct {
		NodeName  string
		GPUID     string
		Point     int64
		podGPUMem int64
	}

	bestNode := candidateNodeGPU{
		Point:     2147483647,
		NodeName:  "",
		GPUID:     "",
		podGPUMem: 0,
	}
	var bestNodeMux sync.Mutex
	tryBestNode := func(point int64, nodeName, GPUID string, gpu_mem int64) {
		bestNodeMux.Lock()
		if point < bestNode.Point {
			bestNode.Point = point
			bestNode.NodeName = nodeName
			bestNode.GPUID = GPUID
			bestNode.podGPUMem = gpu_mem
		}
		bestNodeMux.Unlock()
	}

	var cpuReqTotal, memReqTotal int64 = 0, 0
	for _, container := range sharepod.Spec.Containers {
		cpuReqTotal += container.Resources.Requests.Cpu().MilliValue()
		memReqTotal += container.Resources.Requests.Memory().MilliValue()
	}

	gpu_request_millivalue := int64(math.Ceil(gpu_request * (float64)(1000.0)))

	var wait sync.WaitGroup

	scheduleNode := func(nodeName string,
		nodeRes *NodeResource,
		gpu_request_millivalue int64,
		gpu_request float64,
		gpu_mem int64,
		gpu_mem_set bool) {

		if !gpu_mem_set {
			totalGpuMem := nodeRes.GpuMemTotal
			gpu_mem = int64(math.Ceil(gpu_request * (float64)(totalGpuMem)))
			ksl.Debug("Judge-gpu request: ", gpu_request)
			ksl.Debug("Judge-total gpu mem: ", (totalGpuMem))
			ksl.Debug("Judge-gpu mem: ", gpu_mem)
		}

		if nodeRes.CpuFree < cpuReqTotal || nodeRes.MemFree < memReqTotal {
			wait.Done()
			return
		}
		if isGPUPod {
			findTheHole := false
			for id, gpu := range nodeRes.GpuFree {
				if gpu.GPUFreeReq < gpu_request_millivalue || gpu.GPUFreeMem < gpu_mem {
					continue
				}
				findTheHole = true
				tryBestNode(gpu.GPUFreeReq-gpu_request_millivalue, nodeName, id, gpu_mem)
			}
			if !findTheHole {
				if nodeRes.GpuFreeCount > 0 {
					ksl.Debugf("New GPUID, gpu mem: %v", gpu_mem)
					tryBestNode(1000-gpu_request_millivalue, nodeName, sharedgpuv1.NewGPUID(5), gpu_mem)
				}
			}
			if !gpu_mem_set {
				ksl.Debug("Change Default gpu mem according to gpu_request")
				ksl.Debug("gpu mem: ", gpu_mem)
			}
		} else {
			tryBestNode(nodeRes.CpuFree-cpuReqTotal, nodeName, "", gpu_mem)
		}
		wait.Done()
	}

	wait.Add(len(nodeResources))
	for nodeName, nodeRes := range nodeResources {
		go scheduleNode(nodeName, nodeRes, gpu_request_millivalue, gpu_request, gpu_mem, gpu_mem_set)
	}
	wait.Wait()

	ksl.Debugf("best Point: %v; Node: %v; GPUID: %v, GPUMem: %v, ", bestNode.Point, bestNode.NodeName, bestNode.GPUID, bestNode.podGPUMem)
	return bestNode.NodeName, bestNode.GPUID, bestNode.podGPUMem
}
