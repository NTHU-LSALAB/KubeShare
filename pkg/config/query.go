package config

import (
	"context"
	"time"

	"github.com/prometheus/common/model"
)

// query from prometheus api to get the pod request
// when there are pod updating its status in the cluster
func (c *Config) query() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, warnings, err := c.promeAPI.Query(ctx, GPU_REQUIREMENT, time.Now())
	if err != nil {
		c.ksl.Warnf("Error querying Prometheus: %v\n", err)
		return
	}
	if len(warnings) > 0 {
		c.ksl.Warnf("Warnings: %v\n", warnings)
	}

}

// gpuConfig
// -> key: uuid ; value: all pod request
// podMangerPortConfig
// ->  key: uuid ; value: all pod manager port
func convertData(result model.Value) (gpuConfig, podMangerPortConfig map[string][]string) {
	res := result.(model.Vector)
	//	n := len(res)
	for _, vec := range res {
		uuid := vec.Metric[model.LabelName("uuid")]
		namespace := vec.Metric[model.LabelName("namespace")]
		name := vec.Metric[model.LabelName("pod")]

	}
}

// gpuConfigFile is named by UUID of GPU
// first line means that there are n pod sharing this GPU
// following n lines means that the gpu request of the pods
func writeFile(gpuConfig, podMangerPortConfig map[string]string) {

}
