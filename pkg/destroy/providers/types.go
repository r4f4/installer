package providers

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift/installer/pkg/types"
)

// Destroyer allows multiple implementations of destroy
// for different platforms.
type Destroyer interface {
	GetStages() []Stage
}

// NewFunc is an interface for creating platform-specific destroyers.
type NewFunc func(logger logrus.FieldLogger, metadata *types.ClusterMetadata) (Destroyer, error)

// StageFunc is an interface for functions that run in a destroy Stage
type StageFunc func(context.Context) (bool, error)

// Stage represents a stage in the destroy process, that is, a set of functions
// that can be performed in parallel and has to be completed before moving on
// to the next stage
type Stage struct {
	Name  string
	Funcs []StageFunc
}

func (s *Stage) Run(ctx context.Context) error {
	// FIXME: change this to ExponentialBackOff?
	return wait.PollImmediateUntilWithContext(
		ctx,
		time.Second*10,
		func(ctx context.Context) (done bool, err error) {
			stageFailed := false
			// FIXME: run these in parallel goroutines?
			for _, sfunc := range s.Funcs {
				ok, err := sfunc(ctx)
				// The existence of an error means a non-recoverable condition
				if err != nil {
					return false, err
				}
				// Something failed, so we should retry the stage
				if !ok {
					stageFailed = true
				}
			}
			return !stageFailed, nil
		},
	)
}
