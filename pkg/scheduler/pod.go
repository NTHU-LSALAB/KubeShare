package scheduler

import (
	"fmt"
	"regexp"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	valueFormat, _ = regexp.Compile("[0]+.[0-9]+|[1-9]+[0-9]*[.]+[0]+|[1-9]+")
)

type PodStatus struct {
	namespace string
	name      string
	uid       string //pod uid

	limit    float64
	request  float64
	memory   int64
	model    string
	priority int32

	uuid   string // gpu uuid
	cellID string // cell id

	port         int32
	nodeName     string
	podGroup     string
	minAvailable int
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

// there are 2 types of pods based on priority
// 1. guarantee pod: 1 - 100
// 2. opportunistic pod: -1
// Be careful, the default priority is -1(opportunistic pod)
func (kss *KubeShareScheduler) getPodPrioriy(pod *v1.Pod) (string, bool, int32) {

	msg := ""
	priority, ok := pod.Labels[PodPriority]

	if !ok || len(priority) == 0 {
		msg = fmt.Sprintf("Pod %v/%v: %v is set to -1 and conveted opportunistic pod by default", pod.Namespace, pod.Name, PodPriority)
		kss.ksl.Warnf(msg)
		return msg, true, -1
	}

	p, err := strconv.Atoi(priority)

	if err != nil || p == 0 || p > 1000 || p < -1 {
		msg = fmt.Sprintf("Pod %v/%v: %v set error by user", pod.Namespace, pod.Name, PodPriority)
		kss.ksl.Errorf(msg)
		return msg, false, -1
	}

	return "", true, int32(p)
}

// check the pod whether it needs gpu or not and its correctness.
// return value is error message, correctness, pod status
// there are 3 possible results:
// 	1. the pod need gpu and it is set correct by user
// 	2. the pod need gpu and it is set error by user
// 	3. the pod doesn't need gpu -> regular pod
func (kss *KubeShareScheduler) getPodLabels(pod *v1.Pod) (string, bool, *PodStatus) {
	namespace := pod.Namespace
	name := pod.Name
	key := fmt.Sprintf("%v/%v", namespace, name)
	uid := string(pod.UID)
	kss.podStatusMutex.Lock()
	defer kss.podStatusMutex.Unlock()

	ps, ok := kss.podStatus[key]
	if ok && ps.uid == uid {
		return "", true, ps
	}

	ps = &PodStatus{
		namespace: namespace,
		name:      name,
		uid:       uid,
		nodeName:  pod.Spec.NodeName,
	}

	// get the pod group and min available infomation  and store in pod status
	ps.podGroup, ps.minAvailable = kss.getPodGroupLabels(pod)

	// get the priority and store in pod status
	// if ok equal to false,
	// it means the setting of priority is error
	msg, ok, priority := kss.getPodPrioriy(pod)
	if !ok {
		return msg, false, ps
	}
	ps.priority = priority

	// get label of pod that assigned by user
	labelLimit, okLimit := pod.Labels[PodGPULimit]
	labelRequest, okRequest := pod.Labels[PodGPURequest]
	labelMemory, okMemory := pod.Labels[PodGPUMemory]

	// regular pod
	if !okLimit && !okRequest && !okMemory {
		return "", false, ps
	}

	// check if the limit & request correct or not
	// if a pod need gpu, that must set the limit label
	// correct means that a pod meets the requirement:
	// 1. a pod need <= 1.0 gpu:
	//    + limit >= request
	//    + i.e.
	//    +     limit = 1.0, request = 0.5  (v)
	//    +     limit = 0.5, request = 1.0  (x)
	// 2. a pod need > 1.0 gpu:
	//    + integer number
	//    + limit = request
	//    + i.e.
	//          limit = 2.0, request = 2.0 (v)
	//          limit = 3.0, request = 2.0 (x)
	//          limit = 1.5, request = 1.0 (x)
	formatLimit := valueFormat.FindString(labelLimit)

	if !okLimit || len(formatLimit) != len(labelLimit) {
		msg := fmt.Sprintf("Pod %v/%v: %v set error by user", namespace, name, PodGPULimit)
		kss.ksl.Errorf(msg)
		return msg, false, ps
	}

	limit, err := strconv.ParseFloat(formatLimit, 64)
	if err != nil || limit < 0.0 {
		msg := fmt.Sprintf("Pod %v/%v: %v converted error, %v", namespace, name, PodGPULimit, err)
		kss.ksl.Errorf(msg)
		return msg, false, ps
	}

	request := 0.0

	if okRequest {
		formatRequest := valueFormat.FindString(labelRequest)
		kss.ksl.Debugf("Pod %v/%v: format limit %v, format request %v", namespace, name, formatLimit, formatRequest)
		request, err = strconv.ParseFloat(labelRequest, 64)
		if err != nil || len(formatRequest) != len(labelRequest) ||
			request < 0.0 || (limit > 1.0 && limit != request) ||
			request > limit {
			// kss.ksl.Debugf("err != nil: %v", err != nil)
			// kss.ksl.Debugf("len(formatRequest) != len(labelRequest): %v", len(formatRequest) == len(labelRequest))
			// kss.ksl.Debugf("request < 0.0: %v", request < 0.0)
			// kss.ksl.Debugf("limit > 1.0 && limit != request: %v", (limit > 1.0 && limit != request))
			// kss.ksl.Debugf("request > limit: %v", request > limit)

			msg := fmt.Sprintf("Pod %v/%v: %v set or converted error, %v", namespace, name, PodGPURequest, err)
			kss.ksl.Errorf(msg)
			return msg, false, ps
		}
	}

	// if limit and request equal to 0,
	// it means a pod doesn't need gpu resource in fact.
	// -> regular pod
	if limit == 0.0 && request == 0.0 {
		return "", false, ps
	}

	memory := int64(0)
	if okMemory {
		memory, err = strconv.ParseInt(labelMemory, 10, 64)
		if err != nil || memory < 0 {
			msg := fmt.Sprintf("Pod %v/%v: %v set or converted error, %v", namespace, name, PodGPUMemory, err)
			kss.ksl.Errorf(msg)
			return msg, false, ps
		}
	}

	model := pod.Labels[PodGPUModel]

	ps.limit = limit
	ps.request = request
	ps.memory = memory
	ps.model = model

	kss.podStatus[key] = ps
	return "", true, ps

}

// delete pod status by namespace and name of pod
func (kss *KubeShareScheduler) deletePodRequest(pod *v1.Pod) {
	key := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	kss.podStatusMutex.Lock()
	defer kss.podStatusMutex.Unlock()

	_, ok := kss.podStatus[key]
	if ok {
		delete(kss.podStatus, key)
	}

}

func (kss *KubeShareScheduler) getPodDecision(pod *v1.Pod) {

}
