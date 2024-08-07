package gcp

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"github.com/openshift/installer/pkg/types/gcp"
)

func (o *ClusterUninstaller) listTargetTCPProxies(ctx context.Context, typeName string, listFunc targetTCPProxyListFunc) ([]cloudResource, error) {
	return o.listTargetTCPProxiesWithFilter(ctx, typeName, "items(name),nextPageToken", o.clusterIDFilter(), listFunc)
}

// listTargetTCPProxiesWithFilter lists target TCP Proxies in the project that satisfy the filter criteria.
func (o *ClusterUninstaller) listTargetTCPProxiesWithFilter(ctx context.Context, typeName, fields, filter string, listFunc targetTCPProxyListFunc) ([]cloudResource, error) {
	o.Logger.Debugf("Listing target tcp proxies")
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	result := []cloudResource{}
	list, err := listFunc(ctx, filter, fields)
	if err != nil {
		return nil, fmt.Errorf("failed to list target tcp proxies: %w", err)
	}

	for _, item := range list.Items {
		o.Logger.Debugf("Found target TCP proxy: %s", item.Name)
		result = append(result, cloudResource{
			key:      item.Name,
			name:     item.Name,
			typeName: typeName,
			quota: []gcp.QuotaUsage{{
				Metric: &gcp.Metric{
					Service: gcp.ServiceComputeEngineAPI,
					Limit:   "target_tcp_proxy",
				},
				Amount: 1,
			}},
		})
	}

	return result, nil
}

func (o *ClusterUninstaller) deleteTargetTCPProxy(ctx context.Context, item cloudResource, deleteFunc targetTCPProxyDestroyFunc) error {
	o.Logger.Debugf("Deleting target tcp proxy %s", item.name)
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	op, err := deleteFunc(ctx, item)

	if err != nil && !isNoOp(err) {
		o.resetRequestID(item.typeName, item.name)
		return fmt.Errorf("failed to target tcp proxy %s: %w", item.name, err)
	}
	if op != nil && op.Status == "DONE" && isErrorStatus(op.HttpErrorStatusCode) {
		o.resetRequestID(item.typeName, item.name)
		return errors.Errorf("failed to delete target tcp proxy %s with error: %s", item.name, operationErrorMessage(op))
	}
	if (err != nil && isNoOp(err)) || (op != nil && op.Status == "DONE") {
		o.resetRequestID(item.typeName, item.name)
		o.deletePendingItems(item.typeName, []cloudResource{item})
		o.Logger.Infof("Deleted target tcp proxy %s", item.name)
	}
	return nil
}

// destroyBackendServices removes all backend services resources that have a name prefixed
// with the cluster's infra ID.
func (o *ClusterUninstaller) destroyTargetTCPProxies(ctx context.Context) error {
	for _, ttp := range []targetTCPProxyDestroyer{
		{
			itemTypeName: "targettcpproxy",
			destroyFunc:  o.targetTCPProxyDelete,
			listFunc:     o.targetTCPProxyList,
		},
		{
			itemTypeName: "regiontargettcpproxy",
			destroyFunc:  o.regionTargetTCPProxyDelete,
			listFunc:     o.regionTargetTCPProxyList,
		},
	} {
		found, err := o.listTargetTCPProxies(ctx, ttp.itemTypeName, ttp.listFunc)
		if err != nil {
			return err
		}
		items := o.insertPendingItems(ttp.itemTypeName, found)
		for _, item := range items {
			err := o.deleteTargetTCPProxy(ctx, item, ttp.destroyFunc)
			if err != nil {
				o.errorTracker.suppressWarning(item.key, err, o.Logger)
			}
		}
		if items = o.getPendingItems(ttp.itemTypeName); len(items) > 0 {
			return errors.Errorf("%d items pending", len(items))
		}
	}
	return nil
}

type targetTCPProxyListFunc func(ctx context.Context, filter, fields string) (*compute.TargetTcpProxyList, error)
type targetTCPProxyDestroyFunc func(ctx context.Context, item cloudResource) (*compute.Operation, error)
type targetTCPProxyDestroyer struct {
	itemTypeName string
	destroyFunc  targetTCPProxyDestroyFunc
	listFunc     targetTCPProxyListFunc
}

func (o *ClusterUninstaller) targetTCPProxyDelete(ctx context.Context, item cloudResource) (*compute.Operation, error) {
	return o.computeSvc.TargetTcpProxies.Delete(o.ProjectID, item.name).RequestId(o.requestID(item.typeName, item.name)).Context(ctx).Do()
}

func (o *ClusterUninstaller) targetTCPProxyList(ctx context.Context, filter, fields string) (*compute.TargetTcpProxyList, error) {
	return o.computeSvc.TargetTcpProxies.List(o.ProjectID).Filter(filter).Fields(googleapi.Field(fields)).Context(ctx).Do()
}

func (o *ClusterUninstaller) regionTargetTCPProxyDelete(ctx context.Context, item cloudResource) (*compute.Operation, error) {
	return o.computeSvc.RegionTargetTcpProxies.Delete(o.ProjectID, o.Region, item.name).RequestId(o.requestID(item.typeName, item.name)).Context(ctx).Do()
}

func (o *ClusterUninstaller) regionTargetTCPProxyList(ctx context.Context, filter, fields string) (*compute.TargetTcpProxyList, error) {
	return o.computeSvc.RegionTargetTcpProxies.List(o.ProjectID, o.Region).Filter(filter).Fields(googleapi.Field(fields)).Context(ctx).Do()
}
