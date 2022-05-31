package collector

import (
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var (
	nodeName string
)

type Collector struct {
	ksl    *logrus.Logger
	metric *prometheus.Desc
}

// initialize Collector
func NewCollector(ksl *logrus.Logger) *Collector {

	nodeName = os.Getenv("NODE_NAME")
	ksl.Printf("Node: %v", nodeName)

	return &Collector{
		ksl: ksl,
		//NewDesc(fqName, help string, variableLabels []string, constLabels Labels) *Desc
		metric: prometheus.NewDesc(
			"gpu_capacity",
			"GPU information (in Byte).",
			[]string{"node", "uuid", "model", "memory", "index"},
			nil),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.metric
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	devices := c.getDevices()

	c.ksl.Debugf("Node: %v is updeated at time %v", nodeName, time.Now().Unix())

	for i, device := range devices {
		c.ksl.Debugf("Currently, device %v: %v", i, device.index)
		ch <- prometheus.MustNewConstMetric(
			c.metric,
			prometheus.CounterValue,
			float64(time.Now().Unix()),
			nodeName,
			device.uuid,
			device.model,
			strconv.FormatUint(device.memory, 10),
			strconv.Itoa(device.index))

	}
}
