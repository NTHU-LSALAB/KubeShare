package scheduler

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/common/model"
)

const (
	// query metrics
	GPU_CAPACITY = "gpu_capacity"
)

type GPU struct {
	uuid   string
	memory int64
}

func (kss *KubeShareScheduler) queryGPUCapicity(nodeName string) []model.LabelSet {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, warnings, err := kss.promeAPI.Series(ctx, []string{
		"{__name__=~\"" + GPU_CAPACITY + "\",node=\"" + nodeName + "\"}",
	}, time.Now().Add(-time.Second*10), time.Now())

	if err != nil {
		kss.ksl.Warnf("Error querying Prometheus: %v\n", err)
		return nil
	}
	if len(warnings) > 0 {
		kss.ksl.Warnf("Warnings: %v\n", warnings)
	}
	return result
}

func (kss *KubeShareScheduler) getGPUByNode(nodeName string) {
	results := kss.queryGPUCapicity(nodeName)

	gpuInfos := map[string][]GPU{}
	for _, res := range results {
		uuid := string(res["uuid"])
		model := string(res["model"])
		memory, _ := strconv.ParseInt(string(res["memory"]), 10, 64)
		gpuInfos[model] = append(gpuInfos[model], GPU{
			uuid:   uuid,
			memory: memory,
		})
	}
	kss.gpuInfos[nodeName] = gpuInfos

}
