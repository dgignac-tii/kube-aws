package cmd

import (
	"github.com/kube-aws/kube-aws/logger"
	"github.com/kube-aws/kube-aws/pkg/model"
	"github.com/spf13/cobra"
)

var (
	cmdVersion = &cobra.Command{
		Use:   "version",
		Short: "Print version information and exit",
		Long:  ``,
		Run:   runCmdVersion,
	}
)

func init() {
	RootCmd.AddCommand(cmdVersion)
}

func runCmdVersion(_ *cobra.Command, _ []string) {
	logger.Infof("kube-aws version %s\n", model.VERSION)
}
