package cmd

import (
	"fmt"
	"strings"

	"github.com/kube-aws/kube-aws/core/root"
	"github.com/kube-aws/kube-aws/logger"
	"github.com/spf13/cobra"
)

//TODO this is a first step to calculate the stack cost
//this command could scrap aws to print the total cost, rather just showing the link

var (
	cmdCalculator = &cobra.Command{
		Use:          "calculator",
		Short:        "Discover the monthly cost of your cluster",
		Long:         ``,
		RunE:         runCmdCalculator,
		SilenceUsage: true,
	}

	calculatorOpts = struct {
		profile  string
		awsDebug bool
	}{}
)

func init() {
	RootCmd.AddCommand(cmdCalculator)
	cmdCalculator.Flags().StringVar(&calculatorOpts.profile, "profile", "", "The AWS profile to use from credentials file")
	cmdCalculator.Flags().BoolVar(&calculatorOpts.awsDebug, "aws-debug", false, "Log debug information from aws-sdk-go library")
}

func runCmdCalculator(_ *cobra.Command, _ []string) error {

	opts := root.NewOptions(false, false, calculatorOpts.profile)

	cluster, err := root.LoadClusterFromFile(configPath, opts, calculatorOpts.awsDebug)
	if err != nil {
		return fmt.Errorf("failed to initialize cluster driver: %v", err)
	}

	if _, err := cluster.ValidateStack(); err != nil {
		return fmt.Errorf("error validating cluster: %v", err)
	}

	urls, err := cluster.EstimateCost()

	if err != nil {
		return fmt.Errorf("%v", err)
	}

	logger.Heading("To estimate your monthly cost, open the links below")
	logger.Infof("%v", strings.Join(urls, "\n"))
	return nil
}
