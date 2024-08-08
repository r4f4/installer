package gcp

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"github.com/openshift/installer/pkg/types/gcp"
)

func (o *ClusterUninstaller) listAddresses(ctx context.Context, typeName string, listFunc addressListFunc) ([]cloudResource, error) {
	return o.listAddressesWithFilter(ctx, typeName, "items(name),nextPageToken", o.clusterIDFilter(), listFunc)
}

// listAddressesWithFilter lists addresses in the project that satisfy the filter criteria.
// The fields parameter specifies which fields should be returned in the result, the filter string contains
// a filter string passed to the API to filter results. The listFunc is a client-side function
// that will find all resources that contain the filtered information in the fields supplied.
func (o *ClusterUninstaller) listAddressesWithFilter(ctx context.Context, typeName, fields, filter string, listFunc addressListFunc) ([]cloudResource, error) {
	o.Logger.Debugf("Listing addresses")

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	result := []cloudResource{}
	list, err := listFunc(ctx, filter, fields)
	if err != nil {
		return nil, fmt.Errorf("failed to list addresses: %w", err)
	}

	for _, item := range list.Items {
		o.Logger.Debugf("Found address: %s", item.Name)
		// Note: the string match below is necessary because the gcp is not returning the addressType
		if item.AddressType == "INTERNAL" || strings.Contains(item.Name, "internal") {
			result = append(result, cloudResource{
				key:      item.Name,
				name:     item.Name,
				typeName: typeName,
				quota: []gcp.QuotaUsage{{
					Metric: &gcp.Metric{
						Service: gcp.ServiceComputeEngineAPI,
						Limit:   "internal_addresses",
						Dimensions: map[string]string{
							"region": getNameFromURL("regions", item.Region),
						},
					},
					Amount: 1,
				}},
			})
		}
	}
	return result, nil
}

func (o *ClusterUninstaller) deleteAddress(ctx context.Context, item cloudResource, deleteFunc addressDestroyFunc) error {
	o.Logger.Debugf("Deleting address %s", item.name)
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	op, err := deleteFunc(ctx, item)

	if err != nil && !isNoOp(err) {
		o.resetRequestID(item.typeName, item.name)
		return fmt.Errorf("failed to delete address %s: %w", item.name, err)
	}
	if op != nil && op.Status == "DONE" && isErrorStatus(op.HttpErrorStatusCode) {
		o.resetRequestID(item.typeName, item.name)
		return fmt.Errorf("failed to delete address %s with error: %s", item.name, operationErrorMessage(op))
	}
	if (err != nil && isNoOp(err)) || (op != nil && op.Status == "DONE") {
		o.resetRequestID(item.typeName, item.name)
		o.deletePendingItems(item.typeName, []cloudResource{item})
		o.Logger.Infof("Deleted address %s", item.name)
	}
	return nil
}

// destroyAddresses removes all address resources that have a name prefixed
// with the cluster's infra ID.
func (o *ClusterUninstaller) destroyAddresses(ctx context.Context) error {
	for _, ad := range []addressDestroyer{
		{
			itemTypeName: "address",
			destroyFunc:  o.addressDelete,
			listFunc:     o.addressList,
		},
		{
			itemTypeName: "regionaddress",
			destroyFunc:  o.regionAddressDelete,
			listFunc:     o.regionAddressList,
		},
	} {
		found, err := o.listAddresses(ctx, ad.itemTypeName, ad.listFunc)
		if err != nil {
			return err
		}
		items := o.insertPendingItems(ad.itemTypeName, found)
		for _, item := range items {
			err := o.deleteAddress(ctx, item, ad.destroyFunc)
			if err != nil {
				o.errorTracker.suppressWarning(item.key, err, o.Logger)
			}
		}
		if items = o.getPendingItems(ad.itemTypeName); len(items) > 0 {
			return fmt.Errorf("%d items pending", len(items))
		}
	}
	return nil
}

type addressListFunc func(ctx context.Context, filter, fields string) (*compute.AddressList, error)
type addressDestroyFunc func(ctx context.Context, item cloudResource) (*compute.Operation, error)
type addressDestroyer struct {
	itemTypeName string
	destroyFunc  addressDestroyFunc
	listFunc     addressListFunc
}

func (o *ClusterUninstaller) addressDelete(ctx context.Context, item cloudResource) (*compute.Operation, error) {
	return o.computeSvc.GlobalAddresses.Delete(o.ProjectID, item.name).RequestId(o.requestID(item.typeName, item.name)).Context(ctx).Do()
}

func (o *ClusterUninstaller) addressList(ctx context.Context, filter, fields string) (*compute.AddressList, error) {
	return o.computeSvc.GlobalAddresses.List(o.ProjectID).Filter(filter).Fields(googleapi.Field(fields)).Context(ctx).Do()
}

func (o *ClusterUninstaller) regionAddressDelete(ctx context.Context, item cloudResource) (*compute.Operation, error) {
	return o.computeSvc.Addresses.Delete(o.ProjectID, o.Region, item.name).RequestId(o.requestID(item.typeName, item.name)).Context(ctx).Do()
}

func (o *ClusterUninstaller) regionAddressList(ctx context.Context, filter, fields string) (*compute.AddressList, error) {
	return o.computeSvc.Addresses.List(o.ProjectID, o.Region).Filter(filter).Fields(googleapi.Field(fields)).Context(ctx).Do()
}
