package main

import (
	"log"

	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use: "kubeshare",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:  "scheduler",
			RunE: runScheduler,
		},
		&cobra.Command{
			Use:  "query-ip",
			RunE: runQueryIP,
		},
		&cobra.Command{
			Use:  "config",
			RunE: runConfig,
		},
		&cobra.Command{
			Use:  "collector",
			RunE: runCollector,
		},
		&cobra.Command{
			Use:  "aggregator",
			RunE: runAggregator,
		},
	)

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
