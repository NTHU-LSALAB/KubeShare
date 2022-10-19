package main

import (
	"flag"
	"os"
	"time"

	//kubernetes
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	// prometheus
	"github.com/prometheus/client_golang/api"
	promeV1 "github.com/prometheus/client_golang/api/prometheus/v1"

	// KubeShare
	"github.com/NTHU-LSALAB/KubeShare/pkg/config"
	"github.com/NTHU-LSALAB/KubeShare/pkg/logger"
	"github.com/NTHU-LSALAB/KubeShare/pkg/signals"
)

var (
	// prometheus
	prometheusURL = flag.String("prometheusURL", "", "The address of the prometheus")

	// kubernetes
	masterURL  = flag.String("master", "", "The address of the kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	kubeConfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only requried if out-of-cluster.")

	// logger
	level = flag.Int64("level", 2, "The level order of log.")
)

const (
	// the file storing the log of kubeshare config
	logPath = "kubeshare-config.log"
)

func main() {
	flag.Parse()

	ksl := logger.New(*level, logPath)

	client, err := api.NewClient(api.Config{
		Address: *prometheusURL,
	})
	if err != nil {
		ksl.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	promeAPI := promeV1.NewAPI(client)

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeConfig)
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
}
