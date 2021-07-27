package scheduler

import (
	sharedgpuv1 "KubeShare/pkg/apis/sharedgpu/v1"
	"math"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

var filters = []func(NodeResources, []*corev1.Node, *sharedgpuv1.SharePod){
	GPUAffinityFilter,
	GPUAntiAffinityFilter,
	GPUExclusionFilter,
	NodeSelectorFilter,
	GPUModelFilter,
	GPUMemoryFilter,
}

func GPUAffinityFilter(nodeResources NodeResources, nodeList []*corev1.Node, sharepod *sharedgpuv1.SharePod) {
	affinityTag := ""
	if val, ok := sharepod.ObjectMeta.Annotations[KubeShareScheduleAffinity]; ok {
		affinityTag = val
	} else {
		return
	}
	for _, nodeRes := range nodeResources {
		for GPUID, gpuInfo := range nodeRes.GpuFree {
			notFound := true
			for _, gpuAffinityTag := range gpuInfo.GPUAffinityTags {
				if affinityTag == gpuAffinityTag {
					notFound = false
					break
				}
			}
			if notFound {
				delete(nodeRes.GpuFree, GPUID)
			}
		}
	}
}

func GPUExclusionFilter(nodeResources NodeResources, nodeList []*corev1.Node, sharepod *sharedgpuv1.SharePod) {
	_, ok := sharepod.ObjectMeta.Annotations[KubeShareScheduleExclusion]

	for _, nodeRes := range nodeResources {

		for GPUID, gpuInfo := range nodeRes.GpuFree {
			// vGPU with the label -> delete gpu candidate
			if len(gpuInfo.GPUExclusionTags) != 0 {
				delete(nodeRes.GpuFree, GPUID)
			}
			// vGPU without the labe -> delete gpu candidate
			if len(gpuInfo.GPUExclusionTags) == 0 && ok {
				klog.Info("GPUExclusion: ", gpuInfo.GPUExclusionTags)
				delete(nodeRes.GpuFree, GPUID)
			}
		}
	}
}

func GPUAntiAffinityFilter(nodeResources NodeResources, nodeList []*corev1.Node, sharepod *sharedgpuv1.SharePod) {
	antiAffinityTag := ""
	if val, ok := sharepod.ObjectMeta.Annotations[KubeShareScheduleAntiAffinity]; ok {
		antiAffinityTag = val
	} else {
		return
	}
	for _, nodeRes := range nodeResources {
		for GPUID, gpuInfo := range nodeRes.GpuFree {
			for _, gpuAntiAffinityTag := range gpuInfo.GPUAntiAffinityTags {
				if antiAffinityTag == gpuAntiAffinityTag {
					delete(nodeRes.GpuFree, GPUID)
					break
				}
			}
		}
	}
}

func NodeSelectorFilter(nodeResources NodeResources, nodeList []*corev1.Node, sharepod *sharedgpuv1.SharePod) {

	sharePodLabels := sharepod.Spec.NodeSelector

	for i := range nodeList {
		label := nodeList[i].ObjectMeta.Labels
		for key, val := range sharePodLabels {
			if label[key] != val {
				delete(nodeResources, nodeList[i].Name)
				klog.Infoln("Delete Node: ", nodeList[i].Name)
				break
			}
		}
	}
}

func GPUModelFilter(nodeResources NodeResources, nodeList []*corev1.Node, sharepod *sharedgpuv1.SharePod) {
	gpuModelTag := ""
	if val, ok := sharepod.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPUModel]; ok {
		gpuModelTag = val
	} else {
		return
	}

	for i := range nodeList {
		if nodeGpuModel, ok := nodeList[i].ObjectMeta.Annotations[sharedgpuv1.KubeShareNodeGPUModel]; ok {
			if nodeGpuModel != gpuModelTag {
				delete(nodeResources, nodeList[i].Name)
				klog.Infof("Delete Node %v with gpu card: %v\n", nodeList[i].Name, nodeGpuModel)
			}

		} else {
			delete(nodeResources, nodeList[i].Name)
			klog.Infof("Delete Node %v: can't find gpu model\n", nodeList[i].Name)
		}
	}
}

func GPUMemoryFilter(nodeResources NodeResources, nodeList []*corev1.Node, sharepod *sharedgpuv1.SharePod) {
	gpuMemRequest := int64(0)
	val, ok := sharepod.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPUMemory]
	if ok {
		gpuMemRequestTemp, err := strconv.ParseInt(val, 10, 64)
		if err != nil || gpuMemRequestTemp < 0 {
			return
		}
		gpuMemRequest = gpuMemRequestTemp
	}
	gpu_request := 0.0
	request, request_ok := sharepod.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPURequest]
	if request_ok {
		gpu_request, _ = strconv.ParseFloat(request, 64)
	}
	for nodeName, nodeRes := range nodeResources {
		if !ok && request_ok {
			gpuMemRequest = int64(math.Ceil(gpu_request * (float64)(nodeRes.GpuMemTotal)))
		}
		if nodeRes.GpuMemTotal-gpuMemRequest < 0 {
			delete(nodeResources, nodeName)
			klog.Infoln("Delete Node that gpu memory insufficient: ", nodeName)
			continue
		}

		for GPUID, gpuInfo := range nodeRes.GpuFree {
			if gpuInfo.GPUFreeMem-gpuMemRequest < 0 {
				delete(nodeRes.GpuFree, GPUID)
				klog.Infoln("Delete GPU that gpu memory insufficient: ", GPUID)
			}
		}

	}
}
