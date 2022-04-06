package config

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/common/model"
)

const (
	// query metrics
	GPU_CAPACITY    = "gpu_capacity"
	GPU_REQUIREMENT = "gpu_requirement"
)

// query from prometheus api to get the pod request
// when there are pod updating its status in the cluster
func (c *Config) queryDecision() []model.LabelSet {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, warnings, err := c.promeAPI.Series(ctx, []string{
		"{__name__=~\"" + GPU_REQUIREMENT + "\",node=\"" + nodeName + "\"}",
	}, time.Now().Add(-time.Second*5), time.Now())
	if err != nil {
		c.ksl.Warnf("Error querying Prometheus: %v\n", err)
		return nil
	}
	if len(warnings) > 0 {
		c.ksl.Warnf("Warnings: %v\n", warnings)
	}
	c.ksl.Printf("success")
	return result
}

// gpuConfig
// -> key: uuid ; value: all pod request
// podMangerPortConfig
// ->  key: uuid ; value: all pod manager port
func (c *Config) convertData(result []model.LabelSet) (gpuConfig, podMangerPortConfig map[string][]string) {

	for _, res := range result {
		uuid := string(res["uuid"])

		namespace := res["namespace"]
		name := res["pod"]

		gpuData := fmt.Sprintf("%v/%v %v %v %v\n", namespace, name, res["limit"], res["request"], res["memory"])
		portData := fmt.Sprintf("%v/%v %v\n", namespace, name, res["port"])
		c.ksl.Printf("TEST before")
		if gpuConfig[uuid] == nil {
			gpuConfig[uuid] = make([]string, 0)
			podMangerPortConfig[uuid] = make([]string, 0)
		}
		gpuConfig[uuid] = append(gpuConfig[uuid], gpuData)
		podMangerPortConfig[uuid] = append(podMangerPortConfig[uuid], portData)
		c.ksl.Printf("TEST after")
	}

	return
}

// gpuConfigFile is named by UUID of GPU
// first line means that there are n pod sharing this GPU
// following n lines means that the gpu request of the pods
func (c *Config) writeFile(gpuConfig, podMangerPortConfig map[string][]string) {

	for uuid, gpuRequest := range gpuConfig {
		gpuConfigFile, err := os.Create(schedulerGPUConfigPath + uuid)
		if err != nil {
			c.ksl.Errorf("Error when create config file on path: %s", schedulerGPUConfigPath+uuid)
		}

		gpuConfigFile.WriteString(fmt.Sprintf("%d\n", len(gpuRequest)))
		for _, req := range gpuRequest {
			gpuConfigFile.WriteString(req)
		}
		gpuConfigFile.Sync()
		gpuConfigFile.Close()
	}

	for uuid, managerPort := range podMangerPortConfig {
		podmanagerPortFile, err := os.Create(schedulerGPUPodManagerPortPath + uuid)
		if err != nil {
			c.ksl.Errorf("Error when create config file on path: %s", schedulerGPUPodManagerPortPath+uuid)
		}

		podmanagerPortFile.WriteString(fmt.Sprintf("%d\n", len(managerPort)))
		for _, port := range managerPort {
			podmanagerPortFile.WriteString(port)
		}
		podmanagerPortFile.Sync()
		podmanagerPortFile.Close()
	}
}
