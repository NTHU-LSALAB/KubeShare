package scheduler

import (
	"KubeShare/pkg/lib/queue"
	"bytes"
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	valueFormat, _ = regexp.Compile("[0]+.[0-9]+|[1-9]+[0-9]*[.]+[0]+|[1-9]+")
)

const (
	// the volume path that store the hook library and schedulerIP
	KubeShareLibraryPath = "/kubeshare/library"
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

	uuid         string // gpu uuid
	cells        CellList
	port         int32
	nodeName     string
	podGroup     string
	minAvailable int
}

func (kss *KubeShareScheduler) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	kss.ksl.Infof("[ADD POD] %v/%v(%v)", pod.Namespace, pod.Name, pod.UID)

	if isBound(pod) && !isCompleted(pod) {
		kss.ksl.Infof("[Sync Resource] %v/%v(%v) is bound", pod.Namespace, pod.Name, pod.UID)

		key := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
		ps, ok := kss.podStatus[key]
		if ok {
			kss.ksl.Infof("[Sync Resource] the status of %v/%v(%v) is already exist.", pod.Namespace, pod.Name, pod.UID)
			kss.printPodStatus(ps)
			return
		}
		kss.getOrCreatePodGroupInfo(pod, time.Now())
		// when scheduler is recreating, need to recalculate the resource
		_, needGPU := pod.Annotations[PodGPUMemory]
		// regular pod
		if !needGPU {
			return
		}

		kss.boundPodQueueMutex.Lock()
		defer kss.boundPodQueueMutex.Unlock()
		if kss.boundPodQueue[pod.Spec.NodeName] == nil {
			kss.boundPodQueue[pod.Spec.NodeName] = queue.NewQueue()

		}
		kss.ksl.Infof("[Sync Resource] add pod %v/%v(%v) to bound queue", pod.Namespace, pod.Name, pod.UID)
		kss.boundPodQueue[pod.Spec.NodeName].Enqueue(pod)

	}
}

func (kss *KubeShareScheduler) updatePod(oldObj, newObj interface{}) {
	oldPod := oldObj.(*v1.Pod)
	newPod := newObj.(*v1.Pod)
	kss.ksl.Infof("[UPDATE POD] old: %v/%v(%v) ; new: %v/%v(%v)", oldPod.Namespace, oldPod.Name, oldPod.UID, newPod.Namespace, newPod.Name, newPod.UID)
	if oldPod.UID != newPod.UID {
		kss.deletePod(oldPod)
		kss.addPod(newPod)
		return
	}
}

func (kss *KubeShareScheduler) deletePod(obj interface{}) {
	pod := obj.(*v1.Pod)

	// reclaim the resource and its pod manager pod
	podStatus, ok := kss.deletePodStatus(pod)

	if ok {
		request := podStatus.request
		memory := podStatus.memory
		multiGPU := request > 1.0

		kss.ksl.Infof("[DELETE POD] %v/%v(%v) reclaims its gpu resource", pod.Namespace, pod.Name, pod.UID)

		if multiGPU {
			cells := podStatus.cells
			for _, cell := range cells {
				kss.reclaimResource(cell, cell.leafCellNumber, cell.fullMemory)
			}
		} else {
			port := int(podStatus.port)
			if port >= PodManagerPortStart {
				kss.nodePodManagerPortBitmap[podStatus.nodeName].Unmask(port - PodManagerPortStart)
			}
			if len(podStatus.cells) != 0 {
				cell := podStatus.cells[0]
				kss.reclaimResource(cell, request, memory)
			}
		}

	} else {
		kss.ksl.Infof("[DELETE POD] %v/%v(%v) is a shadow pod or regular pod, not need to reclaim resource", pod.Namespace, pod.Name, pod.UID)
	}

	if podStatus.podGroup != "" {
		key := fmt.Sprintf("%v/%v", pod.Namespace, podStatus.podGroup)
		totalPods := kss.caculateTotalPods(pod.Namespace, podStatus.podGroup)
		if totalPods == 0 {
			kss.podGroupMutex.Lock()
			defer kss.podGroupMutex.Unlock()
			kss.ksl.Warnf("[DELETE PODGROUP] %v, before len %v", key, len(kss.podGroupInfos))
			delete(kss.podGroupInfos, key)
			kss.ksl.Warnf("[DELETE PODGROUP] %v, after len %v and its value %v", key, len(kss.podGroupInfos), kss.podGroupInfos[key])
		}
	}
}

func (kss *KubeShareScheduler) filterPod(obj interface{}) bool {
	switch t := obj.(type) {
	case *v1.Pod:
		pod := obj.(*v1.Pod)
		managed := managedByScheduler(pod)
		if managed && isCompleted(pod) {
			kss.ksl.Infof("pod %v/%v(%v) is %v, reclaim resource", pod.Namespace, pod.Name, pod.UID, pod.Status.Phase)
			kss.deletePod(pod)
		}
		return managed // !isCompleted(pod) && managedByScheduler(pod)
	case cache.DeletedFinalStateUnknown:
		if pod, ok := t.Obj.(*v1.Pod); ok {
			managed := managedByScheduler(pod)
			if managed && isCompleted(pod) {
				kss.ksl.Infof("pod %v/%v(%v) is %v, reclaim resource", pod.Namespace, pod.Name, pod.UID, pod.Status.Phase)
				kss.deletePod(pod)
			}
			return managed //  !isCompleted(pod) && managedByScheduler(pod)
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
// 2. opportunistic pod: 0
// Be careful, the default priority is 0 (opportunistic pod)
func (kss *KubeShareScheduler) getPodPrioriy(pod *v1.Pod) (string, bool, int32) {

	msg := ""
	priority, ok := pod.Labels[PodPriority]

	if !ok || len(priority) == 0 {
		msg = fmt.Sprintf("Pod %v/%v: %v is set to 0 and conveted opportunistic pod by default", pod.Namespace, pod.Name, PodPriority)
		kss.ksl.Warnf(msg)
		return msg, true, 0
	}

	p, err := strconv.Atoi(priority)

	if err != nil || p > 100 || p < -1 {
		msg = fmt.Sprintf("Pod %v/%v: %v set error by user", pod.Namespace, pod.Name, PodPriority)
		kss.ksl.Errorf(msg)
		return msg, false, 0
	}

	return "", true, int32(p)
}

// check the pod whether it needs gpu or not and its correctness.
// return value is error message, correctness, pod status
// there are 3 possible results:
// 	1. the pod need gpu and it is set correct by user
// 	2. the pod need gpu and it is set error by user
// 	3. the pod does not need gpu -> regular pod
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
		//kss.ksl.Debugf("Pod %v/%v: format limit %v, format request %v", namespace, name, formatLimit, formatRequest)
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
	ps.cells = CellList{}

	kss.podStatus[key] = ps
	return "", true, ps

}

// delete pod status by namespace/name of pod
func (kss *KubeShareScheduler) deletePodStatus(pod *v1.Pod) (*PodStatus, bool) {
	key := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	kss.podStatusMutex.Lock()
	defer kss.podStatusMutex.Unlock()

	ps, ok := kss.podStatus[key]
	if ok {
		kss.printPodStatus(ps)
		kss.ksl.Debugf("[deletePodStatus] pod %v(%v) -> it status uid %v", key, pod.UID, ps.uid)
	}
	if ok && ps.uid == string(pod.UID) {
		delete(kss.podStatus, key)
		return ps, true
	}
	return ps, false
}

// reserves the gpu resource and injects the environment variables
func (kss *KubeShareScheduler) newAssumedMultiGPUPod(pod *v1.Pod, nodeName string) *v1.Pod {
	_, _, ps := kss.getPodLabels(pod)
	ps.uid = ""
	cells := ps.cells

	podCopy := pod.DeepCopy()
	if podCopy.Annotations == nil {
		podCopy.Annotations = make(map[string]string)
	}

	var cellIDs, uuids, models bytes.Buffer
	totalMemory := int64(0)
	for _, cell := range cells {
		totalMemory += cell.freeMemory
		kss.reserveResource(cell, cell.available, cell.freeMemory)

		cellIDs.WriteString(cell.id)
		cellIDs.WriteString(",")
		uuids.WriteString(cell.uuid)
		uuids.WriteString(",")
		models.WriteString(cell.cellType)
		models.WriteString(",")
	}

	podCopy.Annotations[PodCellID] = cellIDs.String()
	podCopy.Annotations[PodGPUMemory] = strconv.FormatInt(totalMemory, 10)

	model := models.String()
	podCopy.Annotations[PodGPUModel] = model
	ps.model = model

	uuid := uuids.String()
	podCopy.Annotations[PodGPUUUID] = uuid
	ps.uuid = uuid
	podCopy.ResourceVersion = ""
	podCopy.Spec.NodeName = nodeName
	ps.nodeName = nodeName

	for i := range podCopy.Spec.Containers {
		c := &podCopy.Spec.Containers[i]
		c.Env = append(c.Env,
			v1.EnvVar{
				Name:  "NVIDIA_VISIBLE_DEVICES",
				Value: uuid,
			},
			v1.EnvVar{
				Name:  "NVIDIA_DRIVER_CAPABILITIES",
				Value: "compute,utility",
			})
	}

	return podCopy
}

func (kss *KubeShareScheduler) newAssumedSharedGPUPod(pod *v1.Pod, nodeName string) *v1.Pod {
	_, _, ps := kss.getPodLabels(pod)
	ps.uid = ""
	cell := ps.cells[0]

	podCopy := pod.DeepCopy()
	podCopy.ResourceVersion = ""
	podCopy.Spec.NodeName = nodeName
	ps.nodeName = nodeName
	if podCopy.Annotations == nil {
		podCopy.Annotations = make(map[string]string)
	}

	podCopy.Annotations[PodCellID] = cell.id
	podCopy.Annotations[PodGPUModel] = cell.cellType
	ps.model = cell.cellType

	if ps.memory == 0 {
		ps.memory = int64(math.Floor(ps.request * float64(cell.fullMemory)))
		kss.ksl.Debugf("Defaulting the pod gpu memory request: %v", ps.memory)
	}
	kss.reserveResource(cell, ps.request, ps.memory)
	podCopy.Annotations[PodGPUMemory] = strconv.FormatInt(ps.memory, 10)

	uuid := cell.uuid
	podCopy.Annotations[PodGPUUUID] = uuid
	ps.uuid = uuid

	podManagerPort := kss.nodePodManagerPortBitmap[nodeName].FindNextFromCurrentAndSet() + PodManagerPortStart
	ps.port = int32(podManagerPort)
	podCopy.Annotations[PodManagerPort] = strconv.Itoa(podManagerPort)
	kss.ksl.Debugf("[newAssumedSharedGPUPod] Port: %v", podManagerPort)

	for i := range podCopy.Spec.Containers {
		c := &podCopy.Spec.Containers[i]
		c.Env = append(c.Env,
			v1.EnvVar{
				Name:  "NVIDIA_VISIBLE_DEVICES",
				Value: uuid,
			},
			v1.EnvVar{
				Name:  "NVIDIA_DRIVER_CAPABILITIES",
				Value: "compute,utility",
			},
			v1.EnvVar{
				Name:  "LD_PRELOAD",
				Value: KubeShareLibraryPath + "/libgemhook.so.1",
			},
			v1.EnvVar{
				Name:  "POD_MANAGER_PORT",
				Value: fmt.Sprintf("%d", podManagerPort),
			},
			v1.EnvVar{
				Name:  "POD_NAME",
				Value: fmt.Sprintf("%s/%s", podCopy.Namespace, podCopy.Name),
			})
		c.VolumeMounts = append(c.VolumeMounts,
			v1.VolumeMount{
				Name:      "kubeshare-lib",
				MountPath: KubeShareLibraryPath,
			},
		)
	}
	podCopy.Spec.Volumes = append(podCopy.Spec.Volumes,
		v1.Volume{
			Name: "kubeshare-lib",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: KubeShareLibraryPath,
				},
			},
		},
	)
	return podCopy
}

// update leaf cell and its parents
func (kss *KubeShareScheduler) reserveResource(cell *Cell, request float64, memory int64) {
	kss.ksl.Debugf("==============TEST RESERVE================")
	kss.cellMutex.Lock()
	defer kss.cellMutex.Unlock()

	s := NewStack()
	s.Push(cell)

	//set := map[string]bool{}
	for s.Len() > 0 {
		current := s.Pop()
		current.freeMemory -= memory
		current.available -= request
		// set[current.id] = true
		current.availableWholeCell = math.Floor(current.available)

		kss.ksl.Debugf("[reserveResource] cell : %+v", current)
		if current.parent != nil { //&& !set[cell.parent.id] {
			s.Push(current.parent)
			kss.ksl.Debugf("[reserveResource] parent cell %v:%+v", current.parent.cellType, current.parent)
		}
	}
}

// reclaim leaf cell and its parents
func (kss *KubeShareScheduler) reclaimResource(cell *Cell, request float64, memory int64) {
	kss.ksl.Debugf("==============TEST RECLAIM================")
	kss.cellMutex.Lock()
	defer kss.cellMutex.Unlock()

	s := NewStack()
	s.Push(cell)

	//set := map[string]bool{}
	for s.Len() > 0 {
		current := s.Pop()
		current.freeMemory += memory
		current.available += request
		// set[current.id] = true
		current.availableWholeCell = math.Floor(current.available)

		kss.ksl.Debugf("[reclaimResource] cell : %+v", current)
		if current.parent != nil { //&& !set[cell.parent.id] {
			s.Push(current.parent)
			kss.ksl.Debugf("[reclaimResource] parent cell %v:%+v", current.parent.cellType, current.parent)
		}
	}
}

func (kss *KubeShareScheduler) processBoundPodQueue(nodeName string) {
	kss.ksl.Debugf("[processBoundPodQueue] node: %v", nodeName)
	kss.boundPodQueueMutex.Lock()
	defer kss.boundPodQueueMutex.Unlock()
	q := kss.boundPodQueue[nodeName]

	if q == nil {
		return
	}
	kss.ksl.Debugf("[processBoundPodQueue] node: %v, length of Q: %v", nodeName, q.Len())
	for q.Len() > 0 {
		pod := q.Dequeue().(*v1.Pod)
		if pod.Spec.NodeName == "" {
			continue
		}
		kss.processBoundPod(pod)
	}
}

func (kss *KubeShareScheduler) processBoundPod(pod *v1.Pod) {
	kss.ksl.Debugf("[processBoundPod] pod: %v/%v(%v)", pod.Namespace, pod.Name, pod.UID)
	//key := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	_, _, ps := kss.getPodLabels(pod)
	if ps == nil {
		kss.ksl.Errorf("[processBoundPod] error when get pod status")
		return
	}
	labelMemory := pod.Annotations[PodGPUMemory]
	memory, err := strconv.ParseInt(labelMemory, 10, 64)
	if err != nil {
		kss.ksl.Errorf("[processBoundPod] error when coverting memory, %v", err)
		return
	}

	request := ps.request
	multiGPU := request > 1.0

	if len(ps.cells) == 0 {
		kss.setPodStatus(pod, ps, request, memory)
	}

	if !multiGPU {
		port, err := strconv.Atoi(pod.Annotations[PodManagerPort])
		if err != nil {
			kss.ksl.Errorf("[processBoundPod] error when coverting port, %v", err)
			return
		}
		ps.port = int32(port)
		if port >= PodManagerPortStart {
			kss.nodePodManagerPortBitmap[ps.nodeName].Mask(port - PodManagerPortStart)
		}
	}
	kss.ksl.Infof("[processBoundPod] the status of %v/%v(%v) is set up.", pod.Namespace, pod.Name, pod.UID)
	kss.printPodStatus(ps)
}

func (kss *KubeShareScheduler) setPodStatus(pod *v1.Pod, podStatus *PodStatus, request float64, memory int64) {
	kss.ksl.Errorf("[setPodStatus] pod %v/%v(%v)", pod.Namespace, pod.Name, pod.UID)
	rawUUID := pod.Annotations[PodGPUUUID]
	podStatus.uuid = rawUUID
	uuidList := strings.Split(rawUUID, ",")
	cells := CellList{}

	multiGPU := request > 1.0
	var cellIDs bytes.Buffer

	for _, uuid := range uuidList {
		cell := kss.leafCells[uuid]
		kss.ksl.Debugf("[setPodStatus] Find uuid %v map to cell %v", uuid, cell)
		if cell != nil {
			cells = append(cells, cell)
			if multiGPU {
				kss.reserveResource(cell, cell.leafCellNumber, cell.fullMemory)
			} else {
				kss.reserveResource(cell, request, memory)
			}
			cellIDs.WriteString(cell.id)
			cellIDs.WriteString(",")
		}
	}
	podStatus.cells = cells
	podStatus.memory = memory

	podCopy := pod.DeepCopy()
	podCopy.Annotations[PodCellID] = cellIDs.String()
	podCopy, err := kss.handle.ClientSet().CoreV1().Pods(podCopy.Namespace).Update(context.Background(), podCopy, metav1.UpdateOptions{})
	if err != nil {
		kss.ksl.Errorf("[setPodStatus] Pod %v/%v update cell id error: %v", podCopy.Namespace, podCopy.Name, err)
	}
}
