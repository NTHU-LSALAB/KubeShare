package main

import (
	"flag"

	"KubeShare/pkg/configclient"
	"KubeShare/pkg/logger"

	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	"github.com/sirupsen/logrus"
)

var (
	server string
	level  int64
	ksl    *logrus.Logger
)

const (
	// the file storing the kubeshare config client log
	KubeShareConfigClientLogPath = "kubeshare_config_client.log"
)

func main() {
	flag.Parse()
	ksl = logger.New(level, KubeShareConfigClientLogPath)
	ksl.Info("The level of logger: ", level)

	ksl.Println("Loading NVML")
	if err := nvml.Init(); err != nil {
		ksl.Printf("Failed to initialize NVML: %s.", err)
		ksl.Printf("If this is a GPU node, did you set the docker default runtime to `nvidia`?")
		ksl.Printf("You can check the prerequisites at: https://github.com/NVIDIA/k8s-device-plugin#prerequisites")
		ksl.Printf("You can learn how to set the runtime at: https://github.com/NVIDIA/k8s-device-plugin#quick-start")

		select {}
	}
	defer func() { ksl.Println("Shutdown of NVML returned:", nvml.Shutdown()) }()

	configclient.Run(server, ksl)
}

func init() {
	flag.StringVar(&server, "server-ip", "127.0.0.1:9797", "IP:port of configfile manager.")
	flag.Int64Var(&level, "level", 2, "The level of KubeShare Config Client log.")
}
