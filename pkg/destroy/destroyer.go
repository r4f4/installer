package destroy

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/openshift/installer/pkg/asset/cluster"
	_ "github.com/openshift/installer/pkg/destroy/alibabacloud"
	_ "github.com/openshift/installer/pkg/destroy/aws"
	_ "github.com/openshift/installer/pkg/destroy/azure"
	_ "github.com/openshift/installer/pkg/destroy/baremetal"
	_ "github.com/openshift/installer/pkg/destroy/gcp"
	_ "github.com/openshift/installer/pkg/destroy/ibmcloud"
	_ "github.com/openshift/installer/pkg/destroy/libvirt"
	_ "github.com/openshift/installer/pkg/destroy/nutanix"
	_ "github.com/openshift/installer/pkg/destroy/openstack"
	_ "github.com/openshift/installer/pkg/destroy/ovirt"
	_ "github.com/openshift/installer/pkg/destroy/powervs"
	"github.com/openshift/installer/pkg/destroy/providers"
	_ "github.com/openshift/installer/pkg/destroy/vsphere"
)

// New returns a Destroyer based on `metadata.json` in `rootDir`.
func New(logger logrus.FieldLogger, rootDir string) (providers.Destroyer, error) {
	metadata, err := cluster.LoadMetadata(rootDir)
	if err != nil {
		return nil, err
	}

	platform := metadata.Platform()
	if platform == "" {
		return nil, errors.New("no platform configured in metadata")
	}

	creator, ok := providers.Registry[platform]
	if !ok {
		return nil, errors.Errorf("no destroyers registered for %q", platform)
	}
	return creator(logger, metadata)
}
