package main

import (
	"log"
	"net/http"

	"github.com/NTHU-LSALAB/KubeShare/pkg/collector"
	"github.com/NTHU-LSALAB/KubeShare/pkg/logger"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

type collectorArgs struct {
	MetricsPort string `env:"METRICS_PORT" description:"An port to listen on for web interface and telemetry." default:"9004"`
	MetricsPath string `env:"METRICS_PATH" description:"A path which to expose metrics." default:"/kubeshare-collector"`
	LogLevel    int64  `env:"LOG_LEVEL" description:"The level order of log." default:"2"`
}

func runCollector(_ *cobra.Command, _ []string) error {
	// the file storing the log of kubeshare collector
	const logPath = "kubeshare-collector.log"

	var args collectorArgs
	if _, err := flags.Parse(&args); err != nil {
		log.Fatal(err)
	}

	ksl := logger.New(args.LogLevel, logPath)

	/* load the nvidia toolkit */
	ksl.Println("Loading NVML")
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		ksl.Warnf("Faild to initialize NVML: %s.", nvml.ErrorString(ret))
		ksl.Warnf("If this is a GPU node, did you set the docker default runtime to `nvidia`?")
		ksl.Warnf("You can check the prerequisites at: https://github.com/NVIDIA/k8s-device-plugin#prerequisites")
		ksl.Warnf("You can learn how to set the runtime at: https://github.com/NVIDIA/k8s-device-plugin#quick-start")

		select {}
	}

	defer func() { ksl.Errorln("Shutdown of NVML returned: ", nvml.ErrorString(nvml.Shutdown())) }()

	/* prometheus exporter */
	collector := collector.NewCollector(ksl)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	// expose the registered metrics via HTTP
	http.Handle(args.MetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	ksl.Infof("Starting Server at http://localhost:%s%s", args.MetricsPort, args.MetricsPath)
	ksl.Fatal(http.ListenAndServe(":"+args.MetricsPort, nil))

	return nil
}
