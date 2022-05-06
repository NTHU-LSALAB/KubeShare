package scheduler

import (
	"context"
	"fmt"

	"math"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	// prometheus
	"github.com/prometheus/client_golang/api"
	promeV1 "github.com/prometheus/client_golang/api/prometheus/v1"

	// kubernetes
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	"k8s.io/kubernetes/pkg/scheduler/util"

	// KubeShare
	"KubeShare/pkg/lib/bitmap"
	"KubeShare/pkg/lib/queue"
	"KubeShare/pkg/logger"
	"KubeShare/pkg/signals"
)

const (
	// the name of the plugin used in Registry and configurations
	Name = "kubeshare-scheduler"

	// the file storing the log of kubeshare scheduler
	logPath = "kubeshare-scheduler.log"
	// the file storing physical gpu position
	configPath = "/kubeshare/scheduler/kubeshare-config.yaml"

	PermitWaitingTimeSeconds      = 10
	PodGroupGCIntervalSeconds     = 30
	PodGroupExpirationTimeSeconds = 600
)

var (
	_ framework.QueueSortPlugin = &KubeShareScheduler{}
	_ framework.PreFilterPlugin = &KubeShareScheduler{}
	_ framework.FilterPlugin    = &KubeShareScheduler{}
	_ framework.ReservePlugin   = &KubeShareScheduler{}
	_ framework.UnreservePlugin = &KubeShareScheduler{}
	_ framework.ScorePlugin     = &KubeShareScheduler{}
)

type Args struct {
	// kubernetes
	masterURL  string `json:"master,omitempty"`
	kubeConfig string `json:"kubeconfig,omitempty"`

	// prometheus
	prometheusURL string `json:"prometheusURL,omitempty"`

	// gpu topology configration
	kubeShareConfig string `json:"kubeShareConfig,omitempty"`

	// logger
	level int64 `json:"level,omitempty"`

	// PermitWaitingTime is the wait timeout in seconds.
	PermitWaitingTimeSeconds int64 `json:"permitWaitingTimeSeconds"`
	// PodGroupGCInterval is the period to run gc of PodGroup in seconds.
	PodGroupGCIntervalSeconds int64 `json:"podGroupGCIntervalSeconds"`
	// If the deleted PodGroup stays longer than the PodGroupExpirationTime,
	// the PodGroup will be deleted from PodGroupInfos.
	PodGroupExpirationTimeSeconds int64 `json:"podGroupExpirationTimeSeconds"`
}

type KubeShareScheduler struct {
	// parameters of scheduler
	args      *Args
	handle    framework.FrameworkHandle
	podLister corev1.PodLister
	promeAPI  promeV1.API
	ksl       *logrus.Logger

	// allocation
	gpuPriority                   map[string]int32 // key: model name ; val: priority
	sortGPUByPriority             []string
	gpuInfos                      map[string]map[string][]GPU // key: node name  ; val: {model, all information of gpu in the node}
	cellFreeList                  map[string]LevelCellList
	cellMutex                     *sync.RWMutex
	leafCells                     map[string]*Cell //key: uuid ; val: cell
	leafCellsMutex                *sync.RWMutex
	nodePodManagerPortBitmap      map[string]*bitmap.RRBitmap
	nodePodManagerPortBitmapMutex *sync.Mutex
	// pod group
	// key: <namespace>/<PodGroup name> ; value: *PodGroupInfo.
	podGroupInfos map[string]*PodGroupInfo
	podGroupMutex *sync.RWMutex
	// clock is used to get the current time.
	clock util.Clock
	// pod status
	podStatus      map[string]*PodStatus // key: namespace/name ; value: pod status
	podStatusMutex *sync.RWMutex
	// bound pod queue
	boundPodQueue      map[string]*queue.Queue
	boundPodQueueMutex *sync.RWMutex
}

// initializes a new plugin and returns it
func New(config *runtime.Unknown, handle framework.FrameworkHandle) (framework.Plugin, error) {

	// defaulting argument
	args := &Args{
		level:                         3, // the default level is debugging mode
		prometheusURL:                 "http://prometheus-k8s.monitoring:9090",
		kubeShareConfig:               configPath,
		PermitWaitingTimeSeconds:      PermitWaitingTimeSeconds,
		PodGroupGCIntervalSeconds:     PodGroupGCIntervalSeconds,
		PodGroupExpirationTimeSeconds: PodGroupExpirationTimeSeconds,
	}
	// parse flag
	if err := framework.DecodeInto(config, args); err != nil {
		return nil, err
	}

	// logger
	ksl := logger.New(args.level, logPath)

	// prometheus
	client, err := api.NewClient(api.Config{
		Address: args.prometheusURL,
	})
	if err != nil {
		ksl.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	promeAPI := promeV1.NewAPI(client)

	podLister := handle.SharedInformerFactory().Core().V1().Pods().Lister()
	kss := &KubeShareScheduler{
		args:                          args,
		handle:                        handle,
		promeAPI:                      promeAPI,
		podLister:                     podLister,
		gpuPriority:                   map[string]int32{},
		gpuInfos:                      map[string]map[string][]GPU{},
		cellMutex:                     &sync.RWMutex{},
		nodePodManagerPortBitmap:      map[string]*bitmap.RRBitmap{},
		nodePodManagerPortBitmapMutex: &sync.Mutex{},
		podGroupInfos:                 map[string]*PodGroupInfo{},
		podGroupMutex:                 &sync.RWMutex{},
		podStatus:                     map[string]*PodStatus{},
		podStatusMutex:                &sync.RWMutex{},
		leafCells:                     map[string]*Cell{},
		leafCellsMutex:                &sync.RWMutex{},
		ksl:                           ksl,
		clock:                         util.RealClock{},
		boundPodQueue:                 map[string]*queue.Queue{},
		boundPodQueueMutex:            &sync.RWMutex{},
	}
	// gpu topology
	kubeshareConfig := kss.initRawConfig()
	// ksl.Debugln("=================READ CONFIG=================")
	// ksl.Debugf("%+v", kubeshareConfig)
	kss.watchConfig(kubeshareConfig)

	// ksl.Debugln("=================CELL ELEMENTS=================")
	ce := kss.buildCellChains(kubeshareConfig.CellTypes)
	// for k, v := range ce {
	// 	ksl.Debugf("%+v = %+v", k, v)
	// }
	// ksl.Debugln("=================FREE CELL=================")
	cellFreeList := newCellConstructor(ce, kubeshareConfig.Cells, ksl).build()
	// for k, v := range cellFreeList {
	// 	ksl.Debugf("%+v = ", k)
	// 	for l, cl := range v {
	// 		for i := range cl {
	// 			ksl.Debugf("%+v = %+v", l, cl[i])
	// 		}
	// 	}
	// }

	ksl.Debugln("=================FREE CELL=================")
	ksl.Debugf("size of Free cell: %v", len(cellFreeList))
	for k, v := range cellFreeList {
		ksl.Debugf("%+v = %+v", k, v)
	}

	kss.cellFreeList = cellFreeList

	// try to comment the following two command before run TestPermit
	stopCh := signals.SetupSignalHandler()

	nodeInformer := handle.SharedInformerFactory().Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: kss.isGPUNode,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    kss.addNode,
				UpdateFunc: kss.updateNode,
				DeleteFunc: kss.deleteNode,
			},
		},
	)

	go nodeInformer.Run(stopCh)

	podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	podInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: filterPod,
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: kss.addPod,
				//UpdateFunc: kss.updatePod,
				DeleteFunc: kss.deletePod,
			},
		},
	)
	go podInformer.Run(stopCh)

	if !cache.WaitForCacheSync(
		stopCh,
		nodeInformer.HasSynced,
		podInformer.HasSynced) {
		panic(fmt.Errorf("failed to WaitForCacheSync"))
	}

	go wait.Until(kss.podGroupInfoGC, time.Duration(kss.args.PodGroupGCIntervalSeconds)*time.Second, nil)

	return kss, nil
}

func (kss *KubeShareScheduler) Name() string {
	return Name
}

// sort pods in the scheduling queue.
// 1. compare the priorities of pods
// 2. compare the initialization timestamps of PodGroups/Pods.
// 3. compare the keys of PodGroups/Pods,
//    i.e., if two pods are tied at priority and creation time, the one without podGroup will go ahead of the one with podGroup.
func (kss *KubeShareScheduler) Less(podInfo1, podInfo2 *framework.PodInfo) bool {
	kss.ksl.Debugf("[QueueSort] pod1: %v/%v(%v) v.s. pod2: %v/%v(%v)", podInfo1.Pod.Namespace, podInfo1.Pod.Name, podInfo1.Pod.UID, podInfo2.Pod.Namespace, podInfo2.Pod.Name, podInfo2.Pod.UID)

	pgInfo1 := kss.getOrCreatePodGroupInfo(podInfo1.Pod, podInfo1.InitialAttemptTimestamp)
	pgInfo2 := kss.getOrCreatePodGroupInfo(podInfo2.Pod, podInfo2.InitialAttemptTimestamp)

	priority1 := pgInfo1.priority
	priority2 := pgInfo2.priority

	if priority1 != priority2 {
		return priority1 > priority2
	}

	time1 := pgInfo1.timestamp
	time2 := pgInfo2.timestamp

	if !time1.Equal(time2) {
		return time1.Before(time2)
	}
	return pgInfo1.key < pgInfo2.key
}

// performs the following validations.
// 1. validate if the gpu information `gpu_limit`, `gpu_request`, `gpu_mem` set correct
// 2. validate if the minAvailables and Priorities of all the pods in a PodGroup are the same
// 3. validate if  the total number of pods belonging to the same `PodGroup` is less than `miniAvailable`
// If so, the scheduling process will be interrupted directly to avoid the partial Pods and hold the system resources until timeout.
// It will reduce the overall scheduling time for the whole group.
func (kss *KubeShareScheduler) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	kss.ksl.Infof("[PreFilter] pod: %v/%v(%v) in node %v", pod.Namespace, pod.Name, pod.UID, pod.Spec.NodeName)

	errorMsg, _, ps := kss.getPodLabels(pod)

	// check the labels of a pod is set correctly
	if errorMsg != "" {
		return framework.NewStatus(framework.Unschedulable, errorMsg)
	}

	kss.printPodStatus(ps)

	pgInfo := kss.getOrCreatePodGroupInfo(pod, time.Now())
	pgKey := pgInfo.key

	// check if the pod is regular pod or not
	if len(pgKey) == 0 {
		return framework.NewStatus(framework.Success, "regular pod")
	}

	// check if the minAvailables of pods in same pod group are the same
	pgGroupName, pgMinAvailable := pgInfo.name, pgInfo.minAvailable
	podMinAvailable := ps.minAvailable

	if podMinAvailable != pgMinAvailable {
		msg := fmt.Sprintf("Pod %v/%v(%v) has a different minAvailable (%v) as the PodGroup %v (%v)", pod.Namespace, pod.Name, pod.UID, podMinAvailable, pgGroupName, pgMinAvailable)
		kss.ksl.Errorf(msg)
		return framework.NewStatus(framework.Unschedulable, msg)
	}

	// check if the priorities of pods in same pod group are the same
	pgPriority := pgInfo.priority
	podPriority := ps.priority

	if podPriority != pgPriority {
		msg := fmt.Sprintf("Pod %v/%v(%v) has a different priority (%v) as the PodGroup %v (%v)", pod.Namespace, pod.Name, pod.UID, podPriority, pgGroupName, pgPriority)
		kss.ksl.Errorf(msg)
		return framework.NewStatus(framework.Unschedulable, msg)
	}

	// check if the total pods are less than min available of pod group
	totalPods := kss.caculateTotalPods(pod.Namespace, pgGroupName)
	if totalPods < pgMinAvailable {
		msg := fmt.Sprintf("The count of PodGroup %v (%v) is less than minAvailable(%v) in PreFilter: %d", pgKey, pod.Name, pgMinAvailable, totalPods)
		kss.ksl.Warnf(msg)
		return framework.NewStatus(framework.Unschedulable, msg)
	}

	return framework.NewStatus(framework.Success, "")
}

func (kss *KubeShareScheduler) PreFilterExtensions() framework.PreFilterExtensions {
	kss.ksl.Infof("[PreFilterExtensions]")
	return nil
}

// filter the node that doesn't meet the gpu requirements.
func (kss *KubeShareScheduler) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, node *schedulernodeinfo.NodeInfo) *framework.Status {
	nodeName := node.Node().Name

	kss.processBoundPodQueue(nodeName)

	kss.ksl.Infof("[Filter] pod: %v/%v(%v) in node %v", pod.Namespace, pod.Name, pod.UID, nodeName)

	_, needGPU, ps := kss.getPodLabels(pod)

	// the pod does not need gpu, so we do not need to filter
	if !needGPU {
		return framework.NewStatus(framework.Success, "")
	}

	// check if the port in the node is sufficient
	if kss.nodePodManagerPortBitmap[nodeName] == nil {
		kss.nodePodManagerPortBitmapMutex.Lock()
		defer kss.nodePodManagerPortBitmapMutex.Unlock()
		kss.nodePodManagerPortBitmap[nodeName] = bitmap.NewRRBitmap(512)
		kss.nodePodManagerPortBitmap[nodeName].Mask(0)
	}
	port := kss.nodePodManagerPortBitmap[nodeName].FindNextFromCurrent() + PodManagerPortStart
	kss.ksl.Debugf("[Filter] Port: %v", port)
	if port == -1 {
		msg := fmt.Sprintf("Node %v pod manager port pool is full!", nodeName)
		kss.ksl.Warnf(msg)
		return framework.NewStatus(framework.Unschedulable, msg)
	}

	// get the gpu requirement of the pod
	request := ps.request
	memory := ps.memory

	gpuModelInfos := kss.gpuInfos[nodeName]
	model := ps.model
	assignedGPU := false
	if model != "" {
		assignedGPU = true
	}

	kss.cellMutex.RLock()
	defer kss.cellMutex.RUnlock()
	// check if the node has the specified gpu or not
	if assignedGPU {
		kss.ksl.Infof("[Filter] Pod %v/%v(%v) specified gpu %v", pod.Namespace, pod.Name, pod.UID, model)
		if _, ok := gpuModelInfos[model]; !ok {
			msg := fmt.Sprintf("[Filter] Node %v without the specified gpu %v of pod %v/%v(%v)", nodeName, model, pod.Namespace, pod.Name, pod.UID)
			kss.ksl.Warnf(msg)
			return framework.NewStatus(framework.Unschedulable, msg)
		}

		// check the specified gpu has sufficient gpu resource
		fit, _, _ := kss.filterNode(nodeName, model, request, memory)
		if fit {
			kss.ksl.Infof("[Filter] Node %v meet the gpu requirement of pod %v/%v(%v)", nodeName, pod.Namespace, pod.Name, pod.UID)
			return framework.NewStatus(framework.Success, "")
		} else {
			msg := fmt.Sprintf("[Filter] Node %v doesn't meet the gpu request of pod %v/%v(%v)", nodeName, pod.Namespace, pod.Name, pod.UID)
			kss.ksl.Infof(msg)
			return framework.NewStatus(framework.Unschedulable, msg)
		}

	}

	// filter the node according to its gpu resource
	ok := false
	available := 0.0
	freeMemory := int64(0)
	for model := range gpuModelInfos {

		fit, currentAvailable, currentMemory := kss.filterNode(nodeName, model, request, memory)
		available += currentAvailable
		freeMemory += currentMemory
		if ok = ok || fit; ok || (available >= request && freeMemory >= memory) {
			kss.ksl.Infof("Node %v meet the gpu requirement of pod %v/%v(%v) in Filter", nodeName, pod.Namespace, pod.Name, pod.UID)
			return framework.NewStatus(framework.Success, "")
		}
	}
	msg := fmt.Sprintf("Node %v doesn't meet the gpu request of pod %v/%v(%v) in Filter", nodeName, pod.Namespace, pod.Name, pod.UID)
	kss.ksl.Infof(msg)
	return framework.NewStatus(framework.Unschedulable, msg)
}

// 1. pod not need gpu:
// 		if the node without gpu will set score to 100,
//  	otherwise, 0
// 2. opportunistic pod
// 3. guarantee pod
func (kss *KubeShareScheduler) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	kss.ksl.Infof("[Score] pod: %v/%v(%v) in node %v", pod.Namespace, pod.Name, pod.UID, nodeName)

	_, needGPU, ps := kss.getPodLabels(pod)
	// the pod does not need gpu
	if !needGPU {
		return int64(kss.calculateRegularPodNodeScore(nodeName)), framework.NewStatus(framework.Success, "")
	}

	kss.cellMutex.RLock()
	defer kss.cellMutex.RUnlock()

	score := float64(0)
	// opportunistic pod
	if ps.priority <= 0 {
		score = kss.calculateOpportunisticPodScore(nodeName, ps)
	} else {
		score = kss.calculateGuaranteePodScore(nodeName, ps)
	}
	kss.ksl.Debugf("[Score] Score %v: %v", nodeName, score)
	return int64(score), framework.NewStatus(framework.Success, "")
}

func (kss *KubeShareScheduler) ScoreExtensions() framework.ScoreExtensions {

	kss.ksl.Infof("[ScoreExtensions]")
	return kss
}

func (kss *KubeShareScheduler) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	kss.ksl.Infof("[NormalizeScore] pod: %v/%v(%v)", pod.Namespace, pod.Name, pod.UID)

	var maxScore int64 = math.MinInt64
	var minScore int64 = math.MaxInt64

	for _, node := range scores {
		curScore := int64(node.Score)
		if curScore > maxScore {
			maxScore = curScore
		}
		if curScore < minScore {
			minScore = curScore
		}
	}
	if minScore < 0 {
		reverse := -1 * minScore
		kss.ksl.Debugf("[NormalizeScore] reverse: %v", reverse)
		for i := range scores {
			scores[i].Score += reverse
			kss.ksl.Debugf("[NormalizeScore] reverse Score  %v: %v", scores[i].Name, scores[i].Score)
		}
		maxScore += reverse
		minScore = 0

	}

	if maxScore <= 100 && maxScore >= 0 && minScore <= 100 && minScore >= 0 {
		return nil
	}

	ratio := maxScore - minScore
	defaultRatio := framework.MaxNodeScore - framework.MinNodeScore
	if ratio == 0 {
		ratio = 100
	}
	for i, node := range scores {
		name := scores[i].Name
		kss.ksl.Debugf("Before Score %v: %v", name, scores[i].Score)
		scores[i].Score = node.Score / ratio * defaultRatio
		kss.ksl.Debugf("After Score %v: %v", name, scores[i].Score)
	}

	return nil
}

func (kss *KubeShareScheduler) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	kss.ksl.Infof("[Reserve] pod: %v/%v(%v) in node %v", pod.Namespace, pod.Name, pod.UID, nodeName)

	_, needGPU, ps := kss.getPodLabels(pod)
	if !needGPU {
		return framework.NewStatus(framework.Success, "")
	}

	multiGPU := ps.request > 1.0
	if ps.priority <= 0 {
		ps.cells = kss.calculateOpportunisticPodCellScore(nodeName, ps)
		kss.ksl.Debugf("[Reserve] pod cell for Opportunistic: %+v", ps.cells)
	} else {
		ps.cells = kss.calculateGuaranteePodCellScore(nodeName, ps)
		kss.ksl.Debugf("[Reserve] pod cell for Guarantee: %+v", ps.cells)
	}
	var podCopy *v1.Pod
	if multiGPU {
		podCopy = kss.newAssumedMultiGPUPod(pod, nodeName)

	} else {
		podCopy = kss.newAssumedSharedGPUPod(pod, nodeName)
	}

	err := kss.handle.ClientSet().CoreV1().Pods(podCopy.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
	if err != nil {
		kss.ksl.Debugf("Shadow Pod %v was deleted, %v", pod.Name, err)
	}
	kss.ksl.Infof("[Reserve-> Delete] pod: %v/%v(%v) in node %v", pod.Namespace, pod.Name, pod.UID, pod.Spec.NodeName)

	podCopy, err = kss.handle.ClientSet().CoreV1().Pods(podCopy.Namespace).Create(ctx, podCopy, metav1.CreateOptions{})
	if err != nil {
		kss.ksl.Errorf("Pod %v recreate error: %v", podCopy.Name, err)
	}

	ps.uid = string(podCopy.UID)
	kss.ksl.Debugf("[Reserve] New Pod %v/%v(%v) v.s. Old Pod  %v/%v(%v)", podCopy.Namespace, podCopy.Name, podCopy.UID, pod.Namespace, pod.Name, pod.UID)
	kss.ksl.Infof("[Reserve-> Create] pod: %v/%v(%v) in node %v", podCopy.Namespace, podCopy.Name, podCopy.UID, pod.Spec.NodeName)

	return framework.NewStatus(framework.Success, "")
}

// rejects all other Pods in the PodGroup when one of the pods in the group times out.
func (kss *KubeShareScheduler) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	kss.ksl.Infof("[UnReserve] pod: %v/%v(%v) in node %v", pod.Namespace, pod.Name, pod.UID, nodeName)

	pgInfo := kss.getOrCreatePodGroupInfo(pod, time.Now())
	if pgInfo == nil {
		return
	}

	groupName := pgInfo.name
	kss.handle.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
		if waitingPod.GetPod().Namespace == pod.Namespace && waitingPod.GetPod().Labels[PodGroupName] == groupName {
			kss.ksl.Infof("Reject the pod %v/%v in Unreserve.", groupName, waitingPod.GetPod().Name)
			waitingPod.Reject(kss.Name())
		}
	})

}

func (kss *KubeShareScheduler) Permit(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (*framework.Status, time.Duration) {
	kss.ksl.Infof("[Permit] pod: %v/%v(%v) in node %v", pod.Namespace, pod.Name, pod.UID, nodeName)

	pgInfo := kss.getOrCreatePodGroupInfo(pod, time.Now())
	if pgInfo == nil {
		return framework.NewStatus(framework.Success, ""), 0
	}

	namespace := pod.Namespace
	groupName := pgInfo.name
	minAvailable := pgInfo.minAvailable
	// bound includes both assigned and assumed Pods.
	bound := kss.calculateBoundPods(groupName, namespace)
	// bound is calculated from the snapshot.
	// current pod does not exist in the snapshot during this scheduling cycle.
	current := bound + 1

	if current < minAvailable {
		kss.ksl.Warnf("The count of podGroup %v/%v/%v is not up to minAvailable(%d) in Permit: current(%d)",
			namespace, groupName, pod.Name, minAvailable, current)

		// TODO Change the timeout to a dynamic value depending on the size of the  `PodGroup`
		return framework.NewStatus(framework.Wait, ""), time.Duration(kss.args.PermitWaitingTimeSeconds) * time.Second
	}

	kss.ksl.Infof("The count of PodGroup %v/%v/%v is up to minAvailable(%d) in Permit: current(%d)",
		namespace, groupName, pod.Name, minAvailable, current)

	kss.handle.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
		if waitingPod.GetPod().Namespace == namespace && waitingPod.GetPod().Labels[PodGroupName] == groupName {
			kss.ksl.Infof("Permit allows the pod: %v/%v", groupName, waitingPod.GetPod().Name)
			waitingPod.Allow(kss.Name())
		}
	})

	return framework.NewStatus(framework.Success, ""), 0
}
