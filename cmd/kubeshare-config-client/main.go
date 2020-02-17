package main

import (
	"flag"
	"log"

	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	"k8s.io/klog"
	"github.com/NTHU-LSALAB/KubeShare/pkg/configclient"
)

var (
	server string
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	log.Println("Loading NVML")
	if err := nvml.Init(); err != nil {
		log.Printf("Failed to initialize NVML: %s.", err)
		log.Printf("If this is a GPU node, did you set the docker default runtime to `nvidia`?")
		log.Printf("You can check the prerequisites at: https://github.com/NVIDIA/k8s-device-plugin#prerequisites")
		log.Printf("You can learn how to set the runtime at: https://github.com/NVIDIA/k8s-device-plugin#quick-start")

		select {}
	}
	defer func() { log.Println("Shutdown of NVML returned:", nvml.Shutdown()) }()

	configclient.Run(server)
}

func init() {
	flag.StringVar(&server, "server-ip", "127.0.0.1:9797", "IP:port of configfile manager.")
}
