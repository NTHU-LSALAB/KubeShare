/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	clientset "KubeShare/pkg/client/clientset/versioned"
	informers "KubeShare/pkg/client/informers/externalversions"
	kubesharecontroller "KubeShare/pkg/devicemanager"
	"KubeShare/pkg/logger"
	"KubeShare/pkg/signals"

	"github.com/sirupsen/logrus"
)

const (
	// the file storing the kubeshare device manger log
	KubeShareDeviceMangerLogPath = "kubeshare_device_manager.log"
)

var (
	masterURL  string
	kubeconfig string
	threadNum  int
	level      int64

	ksl *logrus.Logger
)

func main() {
	flag.Parse()
	ksl = logger.New(level, KubeShareDeviceMangerLogPath)
	ksl.Info("The level of logger: ", level)
	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		ksl.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	cfg.QPS = 1024.0
	cfg.Burst = 1024

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		ksl.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	kubeshareClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		ksl.Fatalf("Error building example clientset: %s", err.Error())
	}

	if !checkCRD(kubeshareClient) {
		ksl.Error("CRD doesn't exist. Exiting")
		os.Exit(1)
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	kubeshareInformerFactory := informers.NewSharedInformerFactory(kubeshareClient, time.Second*30)

	controller := kubesharecontroller.NewController(kubeClient, kubeshareClient,
		kubeInformerFactory.Core().V1().Pods(),
		kubeshareInformerFactory.Sharedgpu().V1().SharePods(),
		ksl)

	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	kubeInformerFactory.Start(stopCh)
	kubeshareInformerFactory.Start(stopCh)

	if err = controller.Run(threadNum, stopCh); err != nil {
		ksl.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.IntVar(&threadNum, "threadness", 1, "The number of worker threads.")
	flag.Int64Var(&level, "level", 2, "The level of KubeShare Device Manger log.")
}

func checkCRD(kubeshareClientSet *clientset.Clientset) bool {
	_, err := kubeshareClientSet.SharedgpuV1().SharePods("").List(metav1.ListOptions{})
	if err != nil {
		ksl.Error(err)
		if _, ok := err.(*errors.StatusError); ok {
			if errors.IsNotFound(err) {
				return false
			}
		}
	}
	return true
}
