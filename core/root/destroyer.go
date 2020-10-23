package root

import (
	"fmt"

	"github.com/kube-aws/kube-aws/awsconn"
	"github.com/kube-aws/kube-aws/cfnstack"
	"github.com/kube-aws/kube-aws/core/root/config"
)

type DestroyOptions struct {
	Profile  string
	AwsDebug bool
	Force    bool
}

type ClusterDestroyer interface {
	Destroy() error
}

type clusterDestroyerImpl struct {
	underlying *cfnstack.Destroyer
}

func ClusterDestroyerFromFile(configPath string, opts DestroyOptions) (ClusterDestroyer, error) {
	cfg, err := config.ConfigFromFile(configPath)
	if err != nil {
		return nil, err
	}

	session, err := awsconn.NewSessionFromRegion(cfg.Region, opts.AwsDebug, opts.Profile)
	if err != nil {
		return nil, fmt.Errorf("failed to establish aws session: %v", err)
	}

	cfnDestroyer := cfnstack.NewDestroyer(cfg.RootStackName(), session, cfg.CloudFormation.RoleARN)
	return clusterDestroyerImpl{
		underlying: cfnDestroyer,
	}, nil
}

func (d clusterDestroyerImpl) Destroy() error {
	return d.underlying.Destroy()
}
