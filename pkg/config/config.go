package config

import (
	"strconv"

	"github.com/sirupsen/logrus"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	corev1 "k8s.io/api/core/v1"
	corev1informer "k8s.io/client-go/informers/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

var (
	domain = "sharedgpu/"

	// the upper limit percentage of time over the past sample period that one or more kernels of the pod are executed on the GPU
	KubeShareResourceGPULimit = domain + "gpu_limit"
)

type Config struct {
	ksl           *logrus.Logger
	clientset     kubernetes.Interface
	podLister     corev1lister.PodLister
	prometheusURL *string
}

func NewConfig(ksl *logrus.Logger, clientset kubernetes.Interface, prometheusURL *string, podInformer corev1informer.PodInformer, stopCh <-chan struct{}) *Config {
	config := &Config{
		ksl:           ksl,
		clientset:     clientset,
		prometheusURL: prometheusURL,
		podLister:     podInformer.Lister(),
	}

	pInformer := podInformer.Informer()
	pInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: config.filterPod,
			Handler: cache.ResourceEventHandlerFuncs{
				UpdateFunc: func(old, new interface{}) {

				},
			},
		})

	go pInformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, pInformer.HasSynced) {
		ksl.Fatalf("Failed to WaitForCacheSync")
	}

	return config
}

func (c *Config) filterPod(obj interface{}) bool {
	switch t := obj.(type) {
	case *corev1.Pod:
		return c.checkSharedPod(obj.(*corev1.Pod))
	case cache.DeletedFinalStateUnknown:
		if pod, ok := t.Obj.(*corev1.Pod); ok {
			return c.checkSharedPod(pod)
		}
		return false
	default:
		return false
	}
}

func (c *Config) checkSharedPod(pod *corev1.Pod) bool {
	limit := "0"
	limit = pod.Labels[KubeShareResourceGPULimit]

	if limit != "0" {
		floatLimit, err := strconv.ParseFloat(limit, 64)
		if err != nil {
			c.ksl.Errorf("limit converts error")
		}
		if floatLimit <= 1.0 {
			return true
		}
	}
	return false
}

func query() {

}
