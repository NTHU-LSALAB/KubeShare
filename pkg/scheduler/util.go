package scheduler

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (kss *KubeShareScheduler) printPodStatus(ps *PodStatus) {
	if ps == nil {
		return
	}

	kss.ksl.Debug("------------------------------------")
	kss.ksl.Debugf("Pod %v/%v", ps.namespace, ps.name)
	kss.ksl.Debugf("\tUID:           %v", ps.uid)
	kss.ksl.Debugf("\tLimit:         %v", ps.limit)
	kss.ksl.Debugf("\tRequest:       %v", ps.request)
	kss.ksl.Debugf("\tMemory:        %v", ps.memory)
	kss.ksl.Debugf("\tModel:         %v", ps.model)
	kss.ksl.Debugf("\tPriority:      %v", ps.priority)
	kss.ksl.Debugf("\tUUDI:          %v", ps.uuid)
	kss.ksl.Debugf("\tCellID:        %v", ps.cellID)
	kss.ksl.Debugf("\tPort:          %v", ps.port)
	kss.ksl.Debugf("\tNode Name:     %v", ps.nodeName)
	kss.ksl.Debugf("\tPod Group:     %v", ps.podGroup)
	kss.ksl.Debugf("\tMin Available: %v", ps.minAvailable)
	kss.ksl.Debug("------------------------------------")
}

func (kss *KubeShareScheduler) caculateTotalPods(namespace, podGroupName string) int {
	pods, err := kss.podLister.Pods(namespace).List(labels.Set{PodGroupName: podGroupName}.AsSelector())
	if err != nil {
		kss.ksl.Error(err)
		return 0
	}
	return len(pods)
}

func (kss *KubeShareScheduler) calculateBoundPods(podGroupName, namespace string) int {
	pods, err := kss.handle.SnapshotSharedLister().Pods().FilteredList(func(pod *v1.Pod) bool {
		if pod.Labels[PodGroupName] == podGroupName && pod.Namespace == namespace && pod.Spec.NodeName != "" {
			return true
		}
		return false
	}, labels.NewSelector())
	if err != nil {
		kss.ksl.Error(err)
		return 0
	}
	return len(pods)
}
