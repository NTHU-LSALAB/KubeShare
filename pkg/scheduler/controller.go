package scheduler

import (
	"container/list"
	"fmt"
	"strconv"
	"sync"
	"time"

	//appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	sharedgpuv1 "KubeShare/pkg/apis/sharedgpu/v1"
	clientset "KubeShare/pkg/client/clientset/versioned"
	kubesharescheme "KubeShare/pkg/client/clientset/versioned/scheme"
	informers "KubeShare/pkg/client/informers/externalversions/sharedgpu/v1"
	listers "KubeShare/pkg/client/listers/sharedgpu/v1"

	"github.com/sirupsen/logrus"
)

const controllerAgentName = "kubeshare-scheduler"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Foo is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Foo fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by SharePod"
	// MessageResourceSynced is the message used for an Event fired when a Foo
	// is synced successfully
	MessageResourceSynced = "SharePod scheduled successfully"

	KubeShareScheduleAffinity     = "sharedgpu/sched_affinity"
	KubeShareScheduleAntiAffinity = "sharedgpu/sched_anti-affinity"
	KubeShareScheduleExclusion    = "sharedgpu/sched_exclusion"
)

var (
	ksl *logrus.Logger
)

// Controller is the controller implementation for Foo resources
type Controller struct {
	kubeclientset      kubernetes.Interface
	kubeshareclientset clientset.Interface

	nodesLister     corelisters.NodeLister
	nodesSynced     cache.InformerSynced
	podsLister      corelisters.PodLister
	podsSynced      cache.InformerSynced
	sharepodsLister listers.SharePodLister
	sharepodsSynced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
	recorder  record.EventRecorder

	pendingList    *list.List
	pendingListMux *sync.Mutex
}

func NewController(
	kubeclientset kubernetes.Interface,
	kubeshareclientset clientset.Interface,
	nodeInformer coreinformers.NodeInformer,
	podInformer coreinformers.PodInformer,
	kubeshareInformer informers.SharePodInformer,
	kubeShareLogger *logrus.Logger) *Controller {

	ksl = kubeShareLogger
	utilruntime.Must(kubesharescheme.AddToScheme(scheme.Scheme))
	ksl.Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(ksl.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:      kubeclientset,
		kubeshareclientset: kubeshareclientset,
		nodesLister:        nodeInformer.Lister(),
		nodesSynced:        nodeInformer.Informer().HasSynced,
		podsLister:         podInformer.Lister(),
		podsSynced:         podInformer.Informer().HasSynced,
		sharepodsLister:    kubeshareInformer.Lister(),
		sharepodsSynced:    kubeshareInformer.Informer().HasSynced,
		workqueue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "SharePods"),
		recorder:           recorder,
		pendingList:        list.New(),
		pendingListMux:     &sync.Mutex{},
	}

	ksl.Info("Setting up event handlers")

	kubeshareInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.enqueueSharePod,
		DeleteFunc: controller.resourceChanged,
	})

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: controller.resourceChanged,
	})

	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: controller.resourceChanged,
	})

	return controller
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	ksl.Info("Starting sharedPod controller")

	ksl.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.podsSynced, c.sharepodsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	ksl.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	pendingInsuranceTicker := time.NewTicker(5 * time.Second)
	pendingInsuranceDone := make(chan bool)
	go c.pendingInsurance(pendingInsuranceTicker, &pendingInsuranceDone)

	ksl.Info("Started workers")
	<-stopCh
	ksl.Info("Shutting down workers")
	pendingInsuranceTicker.Stop()
	pendingInsuranceDone <- true

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool

		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}

		if err := c.syncHandler(key); err != nil {
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		c.workqueue.Forget(obj)
		ksl.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller) syncHandler(key string) error {

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	sharepod, err := c.sharepodsLister.SharePods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("SharePod '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	if sharepod.Spec.NodeName != "" {
		utilruntime.HandleError(fmt.Errorf("SharePod '%s' NodeName had been scheduled.", key))
		return nil
	}

	if sharepod, err = c.updateSharePodStatus(sharepod, sharedgpuv1.SharePodPending, "SharePod is creating"); err != nil {
		utilruntime.HandleError(fmt.Errorf("SharePod %s/%s  update error: %v", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, err))
	}
	isGPUPod := false
	gpu_request := 0.0
	gpu_limit := 0.0
	gpu_mem := int64(0)
	gpu_mem_set := false

	request, request_ok := sharepod.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPURequest]
	limit, limit_ok := sharepod.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPULimit]
	memory, memory_ok := sharepod.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPUMemory]
	var errr error

	if limit_ok {
		gpu_limit, err = strconv.ParseFloat(limit, 64)
		if err != nil || gpu_limit > 1.0 || gpu_limit < 0.0 {
			if sharepod, errr = c.updateSharePodStatus(sharepod, sharedgpuv1.SharePodFailed, "The gpu_limit value error"); errr != nil {
				utilruntime.HandleError(fmt.Errorf("SharePod %s/%s update error: %v", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, errr))
			}
			utilruntime.HandleError(fmt.Errorf("SharePod %s/%s value error: %s", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, sharedgpuv1.KubeShareResourceGPULimit))
			return nil
		}
	}

	if request_ok {
		gpu_request, err = strconv.ParseFloat(request, 64)
		if err != nil || (limit_ok && gpu_request > gpu_limit) || gpu_request < 0.0 {
			if sharepod, errr = c.updateSharePodStatus(sharepod, sharedgpuv1.SharePodFailed, "The gpu_request value error"); errr != nil {
				utilruntime.HandleError(fmt.Errorf("SharePod %s/%s update error: %v", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, errr))
			}
			utilruntime.HandleError(fmt.Errorf("SharePod %s/%s value error: %s", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, sharedgpuv1.KubeShareResourceGPURequest))
			return nil
		}
	}

	if memory_ok {
		gpu_mem_set = true
		gpu_mem, err = strconv.ParseInt(memory, 10, 64)
		if err != nil || gpu_mem < 0 {
			if sharepod, errr = c.updateSharePodStatus(sharepod, sharedgpuv1.SharePodFailed, "The gpu_mem value error"); errr != nil {
				utilruntime.HandleError(fmt.Errorf("SharePod %s/%s  update error: %v", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, errr))
			}
			utilruntime.HandleError(fmt.Errorf("SharePod %s/%s value error: %s", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, sharedgpuv1.KubeShareResourceGPUMemory))
			return nil
		}
	}

	if request_ok || limit_ok || memory_ok {
		isGPUPod = true
	}

	if isGPUPod && sharepod.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPUID] != "" {
		utilruntime.HandleError(fmt.Errorf("SharePod '%s' GPUID had been scheduled.", key))
		return nil
	}

	nodeList, err := c.nodesLister.List(labels.Everything())
	if err != nil {
		return err
	}
	podList, err := c.podsLister.List(labels.Everything())
	if err != nil {
		return err
	}
	sharePodList, err := c.sharepodsLister.List(labels.Everything())
	if err != nil {
		return err
	}
	// TODO: when gpu_mem == 0
	schedNode, schedGPUID, gpu_mem := scheduleSharePod(isGPUPod, gpu_request, gpu_mem, gpu_mem_set, sharepod, nodeList, podList, sharePodList)
	if schedNode == "" {
		ksl.Infof("No enough resources for SharePod: %s/%s", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name)

		if sharepod, err = c.updateSharePodStatus(sharepod, sharedgpuv1.SharePodPending, "No enough resources for SharePod"); err != nil {
			utilruntime.HandleError(fmt.Errorf("SharePod %s/%s  update error", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name))
		}

		// return fmt.Errorf("No enough resources for SharePod: %s/%s")
		c.pendingListMux.Lock()
		c.pendingList.PushBack(key)
		c.pendingListMux.Unlock()
		return nil
	} else {
		ksl.Infoln("sharePod: ", sharepod.Namespace, "/", sharepod.Name)
		if request_ok && !memory_ok {
			shp, err := c.updateSharePodGpuMem(sharepod, strconv.FormatInt(gpu_mem, 10))
			sharepod = shp
			if err != nil {
				ksl.Errorf("update annotation error: %v", err)
			}
		}
	}

	ksl.Infof("SharePod '%s' had been scheduled to node '%s' GPUID '%s'.", key, schedNode, schedGPUID)

	if err := c.bindSharePodToNode(sharepod, schedNode, schedGPUID, sharedgpuv1.SharePodPending, "SharePod success to bind to node, wait to create pod"); err != nil {
		if sharepod, err = c.updateSharePodStatus(sharepod, sharedgpuv1.SharePodPending, "Current can not bind to node"); err != nil {
			utilruntime.HandleError(fmt.Errorf("SharePod %s/%s  update error", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name))
		}
		return err
	}

	c.recorder.Event(sharepod, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateSharePodStatus(sharepod *sharedgpuv1.SharePod, phase sharedgpuv1.SharePodPhase, message string) (*sharedgpuv1.SharePod, error) {
	ksl.Debug("updateSharePodStatus")

	sharepodCopy := sharepod.DeepCopy()
	sharepodCopy.Status.Phase = phase
	sharepodCopy.Status.Message = message

	shp, err := c.kubeshareclientset.SharedgpuV1().SharePods(sharepodCopy.Namespace).Update(sharepodCopy)
	return shp, err
}

func (c *Controller) bindSharePodToNode(sharepod *sharedgpuv1.SharePod, schedNode, schedGPUID string, phase sharedgpuv1.SharePodPhase, message string) error {
	sharepodCopy := sharepod.DeepCopy()
	sharepodCopy.Status.Phase = phase
	sharepodCopy.Status.Message = message
	sharepodCopy.Spec.NodeName = schedNode
	if schedGPUID != "" {
		if sharepodCopy.ObjectMeta.Annotations != nil {
			sharepodCopy.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPUID] = schedGPUID
		} else {
			sharepodCopy.ObjectMeta.Annotations = map[string]string{sharedgpuv1.KubeShareResourceGPUID: schedGPUID}
		}
	}

	_, err := c.kubeshareclientset.SharedgpuV1().SharePods(sharepodCopy.Namespace).Update(sharepodCopy)
	return err
}

func (c *Controller) enqueueSharePod(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

func (c *Controller) pendingInsurance(ticker *time.Ticker, done *chan bool) {
	for {
		select {
		case <-(*done):
			return
		case <-ticker.C:
			c.resourceChanged(nil)
		}
	}
}

func (c *Controller) resourceChanged(obj interface{}) {
	// push pending SharePods into workqueue
	c.pendingListMux.Lock()
	for p := c.pendingList.Front(); p != nil; p = p.Next() {
		c.workqueue.Add(p.Value)
	}
	c.pendingList.Init()
	c.pendingListMux.Unlock()
}

func (c *Controller) updateSharePodGpuMem(sharepod *sharedgpuv1.SharePod, gpuMem string) (*sharedgpuv1.SharePod, error) {
	ksl.Debugf("updateSharePodGpuMem")

	sharepodCopy := sharepod.DeepCopy()
	/*if _, exist := sharepod.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPURequest]; exist {
		sharepodCopy.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPUMemory] = gpuMem
	}
	*/
	sharepodCopy.ObjectMeta.Annotations[sharedgpuv1.KubeShareResourceGPUMemory] = gpuMem
	ksl.Debug("sharePod: ", sharepodCopy.Namespace, "/", sharepodCopy.Name)
	for key, val := range sharepodCopy.ObjectMeta.Annotations {
		ksl.Debugf("Annotation=  %v  : %v", key, val)
	}

	shp, err := c.kubeshareclientset.SharedgpuV1().SharePods(sharepodCopy.Namespace).Update(sharepodCopy)
	return shp, err
}
