package main

import (
	"log"
	"os"
	"time"

	"github.com/NTHU-LSALAB/KubeShare/pkg/config"
	"github.com/NTHU-LSALAB/KubeShare/pkg/logger"
	"github.com/NTHU-LSALAB/KubeShare/pkg/signals"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/spf13/cobra"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type configArgs struct {
	PrometheusURL string `long:"prometheusURL" description:"The address of the prometheus" required:"true"`
	MasterURL     string `long:"master" description:"The address of the kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster."`
	KubeConfig    string `long:"kubeconfig" description:"Path to a kubeconfig. Only requried if out-of-cluster."`
	LogLevel      int64  `long:"level" description:"The level order of log." default:"2"`
}

func runConfig(_ *cobra.Command, _ []string) error {
	const logPath = "kubeshare-config.log"

	var args configArgs
	if _, err := flags.Parse(&args); err != nil {
		log.Fatal(err)
	}

	ksl := logger.New(args.LogLevel, logPath)

	client, err := api.NewClient(api.Config{
		Address: args.PrometheusURL,
	})
	if err != nil {
		ksl.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	promeAPI := promv1.NewAPI(client)

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(args.MasterURL, args.KubeConfig)
	if err != nil {
		ksl.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		ksl.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	// Informers are a combination of this event interface and an in-memory cache with indexed lookup.
	// NewSharedInformerFactory caches all objects of a resource in all namespaces in the store
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Second*30)

	config.NewConfig(ksl, promeAPI, clientset, informerFactory.Core().V1().Pods(), stopCh)

	<-stopCh
	ksl.Info("Shutting down config")
	return nil
}
