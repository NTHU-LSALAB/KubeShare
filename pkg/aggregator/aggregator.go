package aggregator

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"k8s.io/client-go/kubernetes"
)

type Aggregator struct {
	ksl       *logrus.Logger
	metric    *prometheus.Desc
	clientset kubernetes.Interface
}

func NewAggregator(ksl *logrus.Logger, clientset kubernetes.Interface) *Aggregator {
	return &Aggregator{
		ksl:       ksl,
		clientset: clientset,
		metric: prometheus.NewDesc(
			"gpu_requirement",
			"GPU requirement of the pod.",
			[]string{
				"namespace",
				"pod",
				"pod_id",
				"node",
				"group_name",
				"min_available",
				"limit",
				"request",
				"memory",
				"cell_id",
				"uuid",
				"port"},
			nil),
	}
}

func (a *Aggregator) Describe(ch chan<- *prometheus.Desc) {
	ch <- a.metric
}

func (a *Aggregator) Collect(ch chan<- prometheus.Metric) {
	podInfos := a.getPods()

	for _, pod := range podInfos {
		ch <- prometheus.MustNewConstMetric(
			a.metric,
			prometheus.CounterValue,
			float64(time.Now().Unix()),
			pod.namespace,
			pod.name,
			pod.podId,
			pod.nodeName,
			pod.groupName,
			pod.minAvailable,
			pod.limit,
			pod.request,
			pod.memory,
			pod.cellID,
			pod.uuid,
			pod.port)
	}
}
