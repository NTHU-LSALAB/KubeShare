package main

import (
	"log"
	"math/rand"
	"time"

	"github.com/NTHU-LSALAB/KubeShare/pkg/scheduler"
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

func runScheduler(_ *cobra.Command, _ []string) error {
	rand.Seed(time.Now().UTC().UnixNano())

	// register custom plugins to the scheduling framework
	c := app.NewSchedulerCommand(
		app.WithPlugin(scheduler.Name, scheduler.New),
	)

	if err := c.Execute(); err != nil {
		log.Fatal(err)
	}

	return nil
}
