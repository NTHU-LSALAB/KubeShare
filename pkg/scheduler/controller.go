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
	"k8s.io/klog"

	kubesharev1 "github.com/NTHU-LSALAB/KubeShare/pkg/apis/kubeshare/v1"
	clientset "github.com/NTHU-LSALAB/KubeShare/pkg/client/clientset/versioned"
	kubesharescheme "github.com/NTHU-LSALAB/KubeShare/pkg/client/clientset/versioned/scheme"
	informers "github.com/NTHU-LSALAB/KubeShare/pkg/client/informers/externalversions/kubeshare/v1"
	listers "github.com/NTHU-LSALAB/KubeShare/pkg/client/listers/kubeshare/v1"
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

	KubeShareScheduleAffinity     = "kubeshare/sched_affinity"
	KubeShareScheduleAntiAffinity = "kubeshare/sched_anti-affinity"
	KubeShareScheduleExclusion    = "kubeshare/sched_exclusion"
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
	kubeshareInformer informers.SharePodInformer) *Controller {

	utilruntime.Must(kubesharescheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
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

	klog.Info("Setting up event handlers")

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

	klog.Info("Starting Foo controller")

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.podsSynced, c.sharepodsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	pendingInsuranceTicker := time.NewTicker(5 * time.Second)
	pendingInsuranceDone := make(chan bool)
	go c.pendingInsurance(pendingInsuranceTicker, &pendingInsuranceDone)

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")
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
		klog.Infof("Successfully synced '%s'", key)
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

	isGPUPod := false
	gpu_request := 0.0
	gpu_limit := 0.0
	gpu_mem := int64(0)

	if sharepod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPURequest] != "" ||
		sharepod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPULimit] != "" ||
		sharepod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPUMemory] != "" {
		var err error
		gpu_limit, err = strconv.ParseFloat(sharepod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPULimit], 64)
		if err != nil || gpu_limit > 1.0 || gpu_limit < 0.0 {
			utilruntime.HandleError(fmt.Errorf("SharePod %s/%s value error: %s", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, kubesharev1.KubeShareResourceGPULimit))
			return nil
		}
		gpu_request, err = strconv.ParseFloat(sharepod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPURequest], 64)
		if err != nil || gpu_request > gpu_limit || gpu_request < 0.0 {
			utilruntime.HandleError(fmt.Errorf("SharePod %s/%s value error: %s", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, kubesharev1.KubeShareResourceGPURequest))
			return nil
		}
		gpu_mem, err = strconv.ParseInt(sharepod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPUMemory], 10, 64)
		if err != nil || gpu_mem < 0 {
			utilruntime.HandleError(fmt.Errorf("SharePod %s/%s value error: %s", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name, kubesharev1.KubeShareResourceGPUMemory))
			return nil
		}
		isGPUPod = true
	}

	if isGPUPod && sharepod.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPUID] != "" {
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
	schedNode, schedGPUID := scheduleSharePod(isGPUPod, gpu_request, gpu_mem, sharepod, nodeList, podList, sharePodList)
	if schedNode == "" {
		klog.Infof("No enough resources for SharePod: %s/%s", sharepod.ObjectMeta.Namespace, sharepod.ObjectMeta.Name)
		// return fmt.Errorf("No enough resources for SharePod: %s/%s")
		c.pendingListMux.Lock()
		c.pendingList.PushBack(key)
		c.pendingListMux.Unlock()
		return nil
	}

	klog.Infof("SharePod '%s' had been scheduled to node '%s' GPUID '%s'.", key, schedNode, schedGPUID)

	if err := c.bindSharePodToNode(sharepod, schedNode, schedGPUID); err != nil {
		return err
	}

	c.recorder.Event(sharepod, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) bindSharePodToNode(sharepod *kubesharev1.SharePod, schedNode, schedGPUID string) error {
	sharepodCopy := sharepod.DeepCopy()
	sharepodCopy.Spec.NodeName = schedNode
	if schedGPUID != "" {
		if sharepodCopy.ObjectMeta.Annotations != nil {
			sharepodCopy.ObjectMeta.Annotations[kubesharev1.KubeShareResourceGPUID] = schedGPUID
		} else {
			sharepodCopy.ObjectMeta.Annotations = map[string]string{kubesharev1.KubeShareResourceGPUID: schedGPUID}
		}
	}

	_, err := c.kubeshareclientset.KubeshareV1().SharePods(sharepodCopy.Namespace).Update(sharepodCopy)
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
