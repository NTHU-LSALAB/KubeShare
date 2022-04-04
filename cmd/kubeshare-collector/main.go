package main

import (
	"flag"
	"net/http"

	// nvidia toolkit
	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"

	// prometheus

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// KubeShare
	"KubeShare/pkg/collector"
	"KubeShare/pkg/logger"
)

var (
	// the parameter from command line
	// web
	listenPort  = flag.String("web.listen-port", "9004", "An port to listen on for web interface and telemetry.")
	metricsPath = flag.String("web.telemetry-path", "/kubeshare-collector", "A path which to expose metrics.")

	// logger
	level = flag.Int64("level", 2, "The level order of log.")
)

const (
	// the file storing the log of kubeshare collector
	logPath = "kubeshare-collector.log"
)

func main() {
	flag.Parse()

	ksl := logger.New(*level, logPath)

	/* load the nvidia toolkit */
	ksl.Println("Loading NVML")
	if err := nvml.Init(); err != nil {
		ksl.Warnf("Faild to initialize NVML: %s.", err)
		ksl.Warnf("If this is a GPU node, did you set the docker default runtime to `nvidia`?")
		ksl.Warnf("You can check the prerequisites at: https://github.com/NVIDIA/k8s-device-plugin#prerequisites")
		ksl.Warnf("You can learn how to set the runtime at: https://github.com/NVIDIA/k8s-device-plugin#quick-start")

		select {}
	}

	defer func() { ksl.Errorln("Shutdown of NVML returned: ", nvml.Shutdown()) }()

	/* prometheus exporter */
	collector := collector.NewCollector(ksl)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	// expose the registered metrics via HTTP
	http.Handle(*metricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	ksl.Infof("Starting Server at http://localhost:%s%s", *listenPort, *metricsPath)
	ksl.Fatal(http.ListenAndServe(":"+*listenPort, nil))
}
