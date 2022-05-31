package collector

import (
	"fmt"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type GPU struct {
	model  string
	uuid   string
	memory uint64
	index  int
}

// func init() {
// 	logfile := "./kubeshare-collector.log"
// 	f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
// 	if err != nil {
// 		log.Fatalf("error opening file: %v", err)
// 	}
// 	//defer f.Close()
// 	log.SetOutput(f)
// }
func (c *Collector) getDevices() []*GPU {
	n, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		fmt.Errorf("Error getting nvidia devices: %v", nvml.ErrorString(ret))
	}

	devices := make([]*GPU, n)

	for i := 0; i < n; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			fmt.Errorf("Error getting device %d: %v", i, nvml.ErrorString(ret))
		}

		migMode, _, ret := device.GetMigMode()
		if ret != nvml.SUCCESS {
			fmt.Errorf("Unable to get mig mode of device at index %d: %v", i, nvml.ErrorString(ret))
		}

		if migMode == 0 {
			uuid, ret := device.GetUUID()
			if ret != nvml.SUCCESS {
				fmt.Errorf("Unable to get uuid of device at index %d: %v", i, nvml.ErrorString(ret))
			}

			memory, ret := device.GetMemoryInfo()
			if ret != nvml.SUCCESS {
				fmt.Errorf("Unable to get memory of device at index %d: %v", i, nvml.ErrorString(ret))
			}

			model, ret := device.GetName()
			if ret != nvml.SUCCESS {
				fmt.Errorf("Unable to get model of device at index %d: %v", i, nvml.ErrorString(ret))
			}
			model = strings.ReplaceAll(model, " ", "-")
			gpu := GPU{
				model:  model,
				uuid:   uuid,
				memory: memory.Total,
				index:  i,
			}
			devices[i] = &gpu
			fmt.Printf("GPU %+v is added.", gpu)
		} else {
			nMig, ret := device.GetMaxMigDeviceCount()
			if ret != nvml.SUCCESS {
				fmt.Errorf("Unable to get number of mig device at index %d: %v", i, nvml.ErrorString(ret))
			}
			for j := 0; j < nMig; j++ {
				migDevice, ret := device.GetMigDeviceHandleByIndex(j)
				if ret != nvml.SUCCESS {
					fmt.Errorf("Error getting mig device %d: %v", j, nvml.ErrorString(ret))
				}
				uuid, ret := migDevice.GetUUID()
				if ret != nvml.SUCCESS {
					fmt.Errorf("Unable to get uuid of mig device at index %d: %v", j, nvml.ErrorString(ret))
				}

				memory, ret := migDevice.GetMemoryInfo()
				if ret != nvml.SUCCESS {
					fmt.Errorf("Unable to get memory of mig device at index %d: %v", j, nvml.ErrorString(ret))
				}

				model, ret := migDevice.GetName()
				if ret != nvml.SUCCESS {
					fmt.Errorf("Unable to get model of mig device at index %d: %v", j, nvml.ErrorString(ret))
				}
				model = strings.ReplaceAll(model, " ", "-")
				gpu := GPU{
					model:  model,
					uuid:   uuid,
					memory: memory.Total,
					index:  i,
				}
				devices[i] = &gpu
				fmt.Printf("GPU %+v is added.", gpu)
			}
		}

	}
	return devices
}

/*
func (c *Collector) getDevices() []*GPU {
	n, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		c.ksl.Fatalf("Error getting nvidia devices: %v", nvml.ErrorString(ret))
	}

	devices := make([]*GPU, n)

	for i := 0; i < n; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			c.ksl.Fatalf("Error getting device %d: %v", i, nvml.ErrorString(ret))
		}

		migMode, _, ret := device.GetMigMode()
		if ret != nvml.SUCCESS {
			c.ksl.Warnf("Unable to get mig mode of device at index %d: %v", i, nvml.ErrorString(ret))
		}

		if migMode == 0 {
			uuid, ret := device.GetUUID()
			if ret != nvml.SUCCESS {
				c.ksl.Fatalf("Unable to get uuid of device at index %d: %v", i, nvml.ErrorString(ret))
			}

			memory, ret := device.GetMemoryInfo()
			if ret != nvml.SUCCESS {
				c.ksl.Fatalf("Unable to get memory of device at index %d: %v", i, nvml.ErrorString(ret))
			}

			model, ret := device.GetName()
			if ret != nvml.SUCCESS {
				c.ksl.Fatalf("Unable to get model of device at index %d: %v", i, nvml.ErrorString(ret))
			}
			device.GetMigDeviceHandleByIndex(0)
			gpu := GPU{
				model:  model,
				uuid:   uuid,
				memory: memory.Total,
			}
			devices[i] = &gpu
			c.ksl.Debugf("GPU %+v is added.", gpu)
		} else {
			nMig, ret := device.GetMaxMigDeviceCount()
			if ret != nvml.SUCCESS {
				c.ksl.Fatalf("Unable to get number of mig device at index %d: %v", i, nvml.ErrorString(ret))
			}
			for j := 0; j < nMig; j++ {
				migDevice, ret := device.GetMigDeviceHandleByIndex(j)
				if ret != nvml.SUCCESS {
					c.ksl.Fatalf("Error getting mig device %d: %v", j, nvml.ErrorString(ret))
				}
				uuid, ret := migDevice.GetUUID()
				if ret != nvml.SUCCESS {
					c.ksl.Fatalf("Unable to get uuid of mig device at index %d: %v", j, nvml.ErrorString(ret))
				}

				memory, ret := migDevice.GetMemoryInfo()
				if ret != nvml.SUCCESS {
					c.ksl.Fatalf("Unable to get memory of mig device at index %d: %v", j, nvml.ErrorString(ret))
				}

				model, ret := migDevice.GetName()
				if ret != nvml.SUCCESS {
					c.ksl.Fatalf("Unable to get model of mig device at index %d: %v", j, nvml.ErrorString(ret))
				}

				gpu := GPU{
					model:  model,
					uuid:   uuid,
					memory: memory.Total,
				}
				devices[i] = &gpu
				c.ksl.Debugf("GPU %+v is added.", gpu)
			}
		}

	}
	return devices
}
*/
