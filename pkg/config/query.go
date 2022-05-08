package config

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

const (
	// query metrics
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

	return result
}

// gpuConfig
// -> key: uuid ; value: all pod request
// podMangerPortConfig
// ->  key: uuid ; value: all pod manager port
func (c *Config) convertData(result []model.LabelSet) (map[string][]string, map[string][]string) {

	gpuConfig, podManagerPortConfig := map[string][]string{}, map[string][]string{}
	for _, res := range result {
		uuid := strings.ReplaceAll(string(res["uuid"]), ",", "")

		namespace := res["exported_namespace"]
		name := res["exported_pod"]

		request, err := strconv.ParseFloat(string(res["request"]), 64)
		if err != nil || request > 1.0 {
			continue
		}
		gpuData := fmt.Sprintf("%v/%v %v %v %v\n", namespace, name, res["limit"], res["request"], res["memory"])
		portData := fmt.Sprintf("%v/%v %v\n", namespace, name, res["port"])

		gpuConfig[uuid] = append(gpuConfig[uuid], gpuData)
		podManagerPortConfig[uuid] = append(podManagerPortConfig[uuid], portData)

	}

	return gpuConfig, podManagerPortConfig
}

// gpuConfigFile is named by UUID of GPU
// first line means that there are n pod sharing this GPU
// following n lines means that the gpu request of the pods
func (c *Config) writeFile(gpuConfig, podManagerPortConfig map[string][]string) {

	for uuid, gpuRequest := range gpuConfig {
		gpuConfigFile, err := os.Create(schedulerGPUConfigPath + uuid)
		if err != nil {
			c.ksl.Errorf("Error when create config file on path: %s, %v", schedulerGPUConfigPath+uuid, err)
		}

		gpuConfigFile.WriteString(fmt.Sprintf("%d\n", len(gpuRequest)))
		for _, req := range gpuRequest {
			gpuConfigFile.WriteString(req)
		}
		gpuConfigFile.Sync()
		gpuConfigFile.Close()
	}

	for uuid, managerPort := range podManagerPortConfig {
		podmanagerPortFile, err := os.Create(schedulerGPUPodManagerPortPath + uuid)
		if err != nil {
			c.ksl.Errorf("Error when create config file on path: %s, %v", schedulerGPUPodManagerPortPath+uuid, err)
		}

		podmanagerPortFile.WriteString(fmt.Sprintf("%d\n", len(managerPort)))
		for _, port := range managerPort {
			podmanagerPortFile.WriteString(port)
		}
		podmanagerPortFile.Sync()
		podmanagerPortFile.Close()
	}

	// TODO: gpuConfig & podManagerConfig == nil
	if len(gpuConfig) == 0 || len(podManagerPortConfig) == 0 {
		c.ksl.Debug("Currently, no pod need gpu, set the file to 0")
		c.cleanFile()
	}
}

func (c *Config) readFileName() []os.FileInfo {
	fileList, err := ioutil.ReadDir(schedulerGPUConfigPath)
	if err != nil {
		c.ksl.Fatalf("Error when read the config file: %v", err)
	}
	return fileList
}

func (c *Config) cleanFile() {
	fileList := c.readFileName()

	for _, uuid := range fileList {
		gpuConfigFile, err := os.Create(schedulerGPUConfigPath + uuid.Name())
		if err != nil {
			c.ksl.Errorf("Error when create config file on path: %s, %v", schedulerGPUConfigPath+uuid.Name(), err)
		}

		gpuConfigFile.WriteString("0\n")
		gpuConfigFile.Sync()
		gpuConfigFile.Close()
	}
	for _, uuid := range fileList {
		podmanagerPortFile, err := os.Create(schedulerGPUPodManagerPortPath + uuid.Name())
		if err != nil {
			c.ksl.Errorf("Error when create config file on path: %s, %v", schedulerGPUPodManagerPortPath+uuid.Name(), err)
		}

		podmanagerPortFile.WriteString("0\n")
		podmanagerPortFile.Sync()
		podmanagerPortFile.Close()
	}
}
