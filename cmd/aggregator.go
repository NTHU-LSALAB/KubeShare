package main

import (
	"log"
	"net/http"

	// prometheus
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	//kubernetes
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	// KubeShare
	"github.com/NTHU-LSALAB/KubeShare/pkg/aggregator"
	"github.com/NTHU-LSALAB/KubeShare/pkg/logger"
)

type aggregatorArgs struct {
	Port        string `long:"web.listen-port" description:"An port to listen on for web interface and telemetry." default:"9005"`
	MetricsPath string `long:"web.telemetry-path" description:"A path which to expose metrics." default:"/kubeshare-aggregator"`
	MasterURL   string `long:"master" description:"The address of the kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster."`
	KubeConfig  string `long:"kubeconfig" description:"Path to a kubeconfig. Only requried if out-of-cluster."`
	LogLevel    int64  `long:"level" description:"The level order of log." default:"2"`
}

func runAggregator(_ *cobra.Command, _ []string) error {
	const logPath = "kubeshare-aggregator.log"

	var args aggregatorArgs
	if _, err := flags.Parse(&args); err != nil {
		log.Fatal(err)
	}

	ksl := logger.New(args.LogLevel, logPath)

	config, err := clientcmd.BuildConfigFromFlags(args.MasterURL, args.KubeConfig)
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
	http.Handle(args.MetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	ksl.Infof("Starting Server at http://localhost:%s%s", args.Port, args.MetricsPath)
	ksl.Fatal(http.ListenAndServe(":"+args.Port, nil))
	return nil
}
