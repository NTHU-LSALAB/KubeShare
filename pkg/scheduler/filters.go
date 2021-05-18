package scheduler

import (
	sharedgpuv1 "KubeShare/pkg/apis/sharedgpu/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

var filters = []func(NodeResources, []*corev1.Node, *sharedgpuv1.SharePod){
	GPUAffinityFilter,
	GPUAntiAffinityFilter,
	GPUExclusionFilter,
	NodeSelectorFilter,
	GPUModelFilter,
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
	exclusionTag := ""
	if val, ok := sharepod.ObjectMeta.Annotations[KubeShareScheduleExclusion]; ok {
		exclusionTag = val
	}
	for _, nodeRes := range nodeResources {
		for GPUID, gpuInfo := range nodeRes.GpuFree {
			if exclusionTag != "" && len(gpuInfo.GPUExclusionTags) == 0 {
				delete(nodeRes.GpuFree, GPUID)
				break
			}
			// len(gpuInfo.GPUExclusionTags) should be only one
			for _, gpuExclusionTag := range gpuInfo.GPUExclusionTags {
				if exclusionTag != gpuExclusionTag {
					delete(nodeRes.GpuFree, GPUID)
					break
				}
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
				klog.Infoln("[RIYACHU] Delete Node: ", nodeList[i].Name)
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
				klog.Infof("[RIYACHU] Delete Node %v with gpu card: %v\n", nodeList[i].Name, nodeGpuModel)
			}

		} else {
			delete(nodeResources, nodeList[i].Name)
			klog.Infof("[RIYACHU] Delete Node %v: can't find gpu model\n", nodeList[i].Name)
		}
	}
}
