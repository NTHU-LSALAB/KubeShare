package scheduler

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

type PodStatus struct {
	namespace string
	name      string
	uid       string //pod uid

	limit   float64
	request float64
	memory  int64
	model   string
	uuid    string // gpu uuid

	cellID string // cell id

	port         int32
	nodeName     string
	podGroup     string
	minAvailable int32
}

func (kss *KubeShareScheduler) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	kss.ksl.Infof("[ADD POD] %v/%v(%v)", pod.Namespace, pod.Name, pod.UID)

	if isBound(pod) {
		kss.ksl.Infof("[Sync Resource] %v/%v(%v) is bound", pod.Namespace, pod.Name, pod.UID)
		//TODO: syncResource
		// when scheduler is recreating, need to recalculate the resource
	}
}

func (kss *KubeShareScheduler) updatePod(oldObj, newObj interface{}) {
	oldPod := oldObj.(*v1.Pod)
	newPod := newObj.(*v1.Pod)
	kss.ksl.Infof("[UPDATE POD] old: %v/%v(%v) ; new: %v/%v(%v)", oldPod.Namespace, oldPod.Name, oldPod.UID, newPod.Namespace, newPod.Name, newPod.UID)
}

func (kss *KubeShareScheduler) deletePod(obj interface{}) {
	pod := obj.(*v1.Pod)
	kss.ksl.Infof("[DELETE POD] %v/%v(%v)", pod.Namespace, pod.Name, pod.UID)
	// reclaim the resource
}

func filterPod(obj interface{}) bool {
	switch t := obj.(type) {
	case *v1.Pod:
		pod := obj.(*v1.Pod)
		return !isCompleted(pod) && managedByScheduler(pod)
	case cache.DeletedFinalStateUnknown:
		if pod, ok := t.Obj.(*v1.Pod); ok {
			return !isCompleted(pod) && managedByScheduler(pod)
		}
		return false
	default:
		return false
	}
}

func isCompleted(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed
}

func managedByScheduler(pod *v1.Pod) bool {
	return pod.Spec.SchedulerName == Name
}

func isBound(pod *v1.Pod) bool {
	return pod.Spec.NodeName != ""
}
