package cmd

import (
	"fmt"
	"os"

	"github.com/kube-aws/kube-aws/core/root"
	"github.com/kube-aws/kube-aws/credential"
	"github.com/kube-aws/kube-aws/logger"
	"github.com/spf13/cobra"
)

var (
	cmdRender = &cobra.Command{
		Use:          "render",
		Short:        "Render deployment artifacts",
		Long:         ``,
		RunE:         runCmdRender,
		SilenceUsage: true,
	}

	cmdRenderCredentials = &cobra.Command{
		Use:          "credentials",
		Short:        "Render credentials",
		Long:         ``,
		RunE:         runCmdRenderCredentials,
		SilenceUsage: true,
	}

	renderCredentialsOpts = credential.GeneratorOptions{}

	cmdRenderStack = &cobra.Command{
		Use:          "stack",
		Short:        "Render CloudFormation stack template and coreos-cloudinit userdata",
		Long:         ``,
		RunE:         runCmdRenderStack,
		SilenceUsage: true,
	}
)

func init() {
	RootCmd.AddCommand(cmdRender)

	cmdRender.AddCommand(cmdRenderCredentials)
	cmdRender.AddCommand(cmdRenderStack)

	cmdRenderCredentials.Flags().BoolVar(&renderCredentialsOpts.GenerateCA, "generate-ca", false, "if generating credentials, generate root CA key and cert. NOT RECOMMENDED FOR PRODUCTION USE- use '-ca-key-path' and '-ca-cert-path' options to provide your own certificate authority assets")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.CaKeyPath, "ca-key-path", "./credentials/ca-key.pem", "path to pem-encoded CA RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.CommonName, "cn", "kube-ca", "FQDN for CN in the self-generate CA certificate")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.CaCertPath, "ca-cert-path", "./credentials/ca.pem", "path to pem-encoded CA x509 certificate")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.AdminKeyPath, "admin-key-path", "", "path to pem-encoded CA RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.ApiServerAggregatorKeyPath, "apiserver-aggregator-key-path", "", "path to pem-encoded apiserver aggregator RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.ApiServerKeyPath, "apiserver-key-path", "", "path to pem-encoded apiserver RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.EtcdClientKeyPath, "etcd-client-key-path", "", "path to pem-encoded etcd client RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.EtcdKeyPath, "etcd-key-path", "", "path to pem-encoded etcd RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.KubeControllerManagerKeyPath, "kube-controller-manager-key-path", "", "path to pem-encoded kube controller manager RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.KubeSchedulerKeyPath, "kube-scheduler-key-path", "", "path to pem-encoded kube scheduler RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.ServiceAccountKeyPath, "service-account-key-path", "", "path to pem-encoded service account RSA key")
	cmdRenderCredentials.Flags().StringVar(&renderCredentialsOpts.WorkerKeyPath, "worker-key-path", "", "path to pem-encoded worker RSA key")
	cmdRenderCredentials.Flags().BoolVar(&renderCredentialsOpts.AwsDebug, "aws-debug", false, "Log debug information from aws-sdk-go library")

}

func runCmdRender(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("render takes no arguments\n")
	}

	if err := runCmdRenderStack(cmdRenderCredentials, args); err != nil {
		return err
	}

	if _, err := os.Stat(renderCredentialsOpts.CaKeyPath); os.IsNotExist(err) {
		renderCredentialsOpts.GenerateCA = true
	}
	if err := runCmdRenderCredentials(cmdRenderCredentials, args); err != nil {
		return err
	}

	return nil
}

func runCmdRenderStack(_ *cobra.Command, _ []string) error {
	if err := root.RenderStack(configPath); err != nil {
		return err
	}

	successMsg :=
		`Success! Stack rendered to ./stack-templates.

Next steps:
1. (Optional) Validate your changes to %s with "kube-aws validate"
2. (Optional) Further customize the cluster by modifying templates in ./stack-templates or cloud-configs in ./userdata.
3. Start the cluster with "kube-aws apply".
`

	logger.Infof(successMsg, configPath)
	return nil
}

func runCmdRenderCredentials(_ *cobra.Command, _ []string) error {
	if _, err := os.Stat(renderCredentialsOpts.CaKeyPath); os.IsNotExist(err) {
		renderCredentialsOpts.GenerateCA = true
	}
	return root.RenderCredentials(configPath, renderCredentialsOpts)
}
