package api

import "github.com/kube-aws/kube-aws/provisioner"

type HelmReleaseFileset struct {
	ValuesFile  *provisioner.RemoteFile
	ReleaseFile *provisioner.RemoteFile
}
