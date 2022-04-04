package collector

import (
	// nvidia toolkit
	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
)

type GPU struct {
	model  string
	uuid   string
	memory uint64
}

func (c *Collector) getDevices() []*GPU {
	n, err := nvml.GetDeviceCount()
	if err != nil {
		c.ksl.Fatalf("Error getting nvidia devices: %v", err)
	}

	devices := make([]*GPU, n)

	for i := uint(0); i < n; i++ {
		device, err := nvml.NewDevice(i)
		if err != nil {
			c.ksl.Fatalf("Error getting device %d: %v", i, err)
		}

		gpu := GPU{
			model:  *device.Model,
			uuid:   device.UUID,
			memory: *device.Memory * 1048576, //1024*1024
		}
		devices[i] = &gpu
		c.ksl.Debugf("GPU %+v is added.", gpu)
	}
	return devices
}
