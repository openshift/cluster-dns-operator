package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/sirupsen/logrus"
)

func main() {
	var rootCmd = &cobra.Command{Use: "dns-operator"}
	rootCmd.AddCommand(NewStartCommand())
	rootCmd.AddCommand(NewUpdateHostsCommand())

	if err := rootCmd.Execute(); err != nil {
		logrus.Error(err, "error")
		os.Exit(1)
	}
}
