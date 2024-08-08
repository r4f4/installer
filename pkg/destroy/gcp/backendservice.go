package gcp

import (
	"context"
	"fmt"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"github.com/openshift/installer/pkg/types/gcp"
)

func (o *ClusterUninstaller) listBackendServices(ctx context.Context, typeName string, listFunc backendServiceListFunc) ([]cloudResource, error) {
	return o.listBackendServicesWithFilter(ctx, typeName, "items(name),nextPageToken", o.clusterIDFilter(), listFunc)
}

// listBackendServicesWithFilter lists backend services in the project that satisfy the filter criteria.
// The fields parameter specifies which fields should be returned in the result, the filter string contains
// a filter string passed to the API to filter results. The listFunc is a client-side function
// // that will find all resources that contain the filtered information in the fields supplied.
func (o *ClusterUninstaller) listBackendServicesWithFilter(ctx context.Context, typeName, fields, filter string, listFunc backendServiceListFunc) ([]cloudResource, error) {
	o.Logger.Debugf("Listing backend services")

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	result := []cloudResource{}
	list, err := listFunc(ctx, filter, fields)
	if err != nil {
		return nil, fmt.Errorf("failed to list backend services: %w", err)
	}

	for _, item := range list.Items {
		o.Logger.Debugf("Found backend service: %s", item.Name)
		result = append(result, cloudResource{
			key:      item.Name,
			name:     item.Name,
			typeName: typeName,
			quota: []gcp.QuotaUsage{{
				Metric: &gcp.Metric{
					Service: gcp.ServiceComputeEngineAPI,
					Limit:   "backend_services",
				},
				Amount: 1,
			}},
		})
	}
	return result, nil
}

func (o *ClusterUninstaller) deleteBackendService(ctx context.Context, item cloudResource, deleteFunc backendServiceDestroyFunc) error {
	o.Logger.Debugf("Deleting backend service %s", item.name)
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	op, err := deleteFunc(ctx, item)

	if err != nil && !isNoOp(err) {
		o.resetRequestID(item.typeName, item.name)
		return fmt.Errorf("failed to delete backend service %s: %w", item.name, err)
	}
	if op != nil && op.Status == "DONE" && isErrorStatus(op.HttpErrorStatusCode) {
		o.resetRequestID(item.typeName, item.name)
		return fmt.Errorf("failed to delete backend service %s with error: %s", item.name, operationErrorMessage(op))
	}
	if (err != nil && isNoOp(err)) || (op != nil && op.Status == "DONE") {
		o.resetRequestID(item.typeName, item.name)
		o.deletePendingItems(item.typeName, []cloudResource{item})
		o.Logger.Infof("Deleted backend service %s", item.name)
	}
	return nil
}

// destroyBackendServices removes all backend services resources that have a name prefixed
// with the cluster's infra ID.
func (o *ClusterUninstaller) destroyBackendServices(ctx context.Context) error {
	for _, dbs := range []backendServiceDestroyer{
		{
			itemTypeName: "backendservice",
			destroyFunc:  o.backendServiceDelete,
			listFunc:     o.backendServiceList,
		},
		{
			itemTypeName: "regionBackendService",
			destroyFunc:  o.regionBackendServiceDelete,
			listFunc:     o.regionBackendServiceList,
		},
	} {
		found, err := o.listBackendServices(ctx, dbs.itemTypeName, dbs.listFunc)
		if err != nil {
			return err
		}
		items := o.insertPendingItems(dbs.itemTypeName, found)
		for _, item := range items {
			err := o.deleteBackendService(ctx, item, dbs.destroyFunc)
			if err != nil {
				o.errorTracker.suppressWarning(item.key, err, o.Logger)
			}
		}
		if items = o.getPendingItems(dbs.itemTypeName); len(items) > 0 {
			return fmt.Errorf("%d items pending", len(items))
		}
	}
	return nil
}

type backendServiceListFunc func(ctx context.Context, filter, fields string) (*compute.BackendServiceList, error)
type backendServiceDestroyFunc func(ctx context.Context, item cloudResource) (*compute.Operation, error)
type backendServiceDestroyer struct {
	itemTypeName string
	destroyFunc  backendServiceDestroyFunc
	listFunc     backendServiceListFunc
}

func (o *ClusterUninstaller) backendServiceDelete(ctx context.Context, item cloudResource) (*compute.Operation, error) {
	return o.computeSvc.BackendServices.Delete(o.ProjectID, item.name).RequestId(o.requestID(item.typeName, item.name)).Context(ctx).Do()
}

func (o *ClusterUninstaller) backendServiceList(ctx context.Context, filter, fields string) (*compute.BackendServiceList, error) {
	return o.computeSvc.BackendServices.List(o.ProjectID).Filter(filter).Fields(googleapi.Field(fields)).Context(ctx).Do()
}

func (o *ClusterUninstaller) regionBackendServiceDelete(ctx context.Context, item cloudResource) (*compute.Operation, error) {
	return o.computeSvc.RegionBackendServices.Delete(o.ProjectID, o.Region, item.name).RequestId(o.requestID(item.typeName, item.name)).Context(ctx).Do()
}

func (o *ClusterUninstaller) regionBackendServiceList(ctx context.Context, filter, fields string) (*compute.BackendServiceList, error) {
	return o.computeSvc.RegionBackendServices.List(o.ProjectID, o.Region).Filter(filter).Fields(googleapi.Field(fields)).Context(ctx).Do()
}
