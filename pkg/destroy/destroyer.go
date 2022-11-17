package destroy

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/openshift/installer/pkg/asset/cluster"
	// _ "github.com/openshift/installer/pkg/destroy/alibabacloud"
	// _ "github.com/openshift/installer/pkg/destroy/aws"
	// _ "github.com/openshift/installer/pkg/destroy/azure"
	// _ "github.com/openshift/installer/pkg/destroy/baremetal"
	// _ "github.com/openshift/installer/pkg/destroy/gcp"
	// _ "github.com/openshift/installer/pkg/destroy/ibmcloud"
	// _ "github.com/openshift/installer/pkg/destroy/libvirt"
	// _ "github.com/openshift/installer/pkg/destroy/nutanix"
	// _ "github.com/openshift/installer/pkg/destroy/openstack"
	// _ "github.com/openshift/installer/pkg/destroy/ovirt"
	// _ "github.com/openshift/installer/pkg/destroy/powervs"
	"github.com/openshift/installer/pkg/destroy/providers"
	_ "github.com/openshift/installer/pkg/destroy/vsphere"
	"github.com/openshift/installer/pkg/types"
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

func Run(o providers.Destroyer, logger logrus.FieldLogger) (*types.ClusterQuota, error) {
	waitCtx := context.Background()
	for _, stage := range o.GetStages() {
		logger := logger.WithField("stage", stage.Name)
		logger.Debugln("Running")

		// We only proceed to the next stages when the current one succeeds. If
		// a non-recoverable error happens, then the whole destroy is failed
		if err := stage.Run(waitCtx); err != nil {
			// logger.Debugln("Failed")
			return nil, errors.Wrapf(err, "stage '%s' failed", stage.Name)
		}
		logger.Infoln("Success")
	}
	return nil, nil
}
