package main

import (
	"flag"
	"net/http"

	// prometheus
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	//kubernetes
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	// KubeShare
	"github.com/NTHU-LSALAB/KubeShare/pkg/aggregator"
	"github.com/NTHU-LSALAB/KubeShare/pkg/logger"
)

var (
	// the parameter from command line
	// web
	listenPort  = flag.String("web.listen-port", "9005", "An port to listen on for web interface and telemetry.")
	metricsPath = flag.String("web.telemetry-path", "/kubeshare-aggregator", "A path which to expose metrics.")

	// kubernetes
	masterURL  = flag.String("master", "", "The address of the kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeConfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only requried if out-of-cluster.")

	// logger
	level = flag.Int64("level", 2, "The level order of log.")
)

const (
	// the file storing the log of kubeshare aggregator
	logPath = "kubeshare-aggregator.log"
)

func main() {
	flag.Parse()

	ksl := logger.New(*level, logPath)

	config, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeConfig)
	if err != nil {
		ksl.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		ksl.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	/* prometheus exporter */
	aggregator := aggregator.NewAggregator(ksl, clientset)

	registry := prometheus.NewRegistry()
	registry.MustRegister(aggregator)
	// expose the registered metrics via HTTP
	http.Handle(*metricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	ksl.Infof("Starting Server at http://localhost:%s%s", *listenPort, *metricsPath)
	ksl.Fatal(http.ListenAndServe(":"+*listenPort, nil))
}
