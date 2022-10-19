package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

func runQueryIP(_ *cobra.Command, _ []string) error {
	const (
		schedulerIPPath    = "/kubeshare/library/schedulerIP.txt"
		schedulerIPEnvName = "KUBESHARE_SCHEDULER_IP"
	)

	f, err := os.Create(schedulerIPPath)
	if err != nil {
		log.Fatalf("Error when create scheduler ip file on path: %s", schedulerIPPath)
	}
	f.WriteString(os.Getenv(schedulerIPEnvName) + "\n")
	f.Sync()
	f.Close()
	return nil
}
