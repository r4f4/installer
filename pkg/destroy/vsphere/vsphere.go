package vsphere

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/installer/pkg/asset/installconfig/vsphere"
	"github.com/openshift/installer/pkg/destroy/providers"
	installertypes "github.com/openshift/installer/pkg/types"
	vspheretypes "github.com/openshift/installer/pkg/types/vsphere"
)

var defaultTimeout = time.Minute * 5

// ClusterUninstaller holds the various options for the cluster we want to delete.
type ClusterUninstaller struct {
	ClusterID         string
	InfraID           string
	vCenter           string
	username          string
	password          string
	terraformPlatform string

	Client     *vim25.Client
	RestClient *rest.Client

	Logger logrus.FieldLogger

	context       context.Context
	clientCleanup vsphere.ClientLogout
}

// New returns an VSphere destroyer from ClusterMetadata.
func New(logger logrus.FieldLogger, metadata *installertypes.ClusterMetadata) (providers.Destroyer, error) {
	vim25Client, restClient, cleanup, err := vsphere.CreateVSphereClients(context.TODO(),
		metadata.VSphere.VCenter,
		metadata.VSphere.Username,
		metadata.VSphere.Password)
	if err != nil {
		return nil, err
	}
	// defer cleanup()

	return &ClusterUninstaller{
		ClusterID:         metadata.ClusterID,
		InfraID:           metadata.InfraID,
		vCenter:           metadata.VSphere.VCenter,
		username:          metadata.VSphere.Username,
		password:          metadata.VSphere.Password,
		terraformPlatform: metadata.VSphere.TerraformPlatform,
		RestClient:        restClient,
		Client:            vim25Client,

		Logger:        logger,
		context:       context.Background(),
		clientCleanup: cleanup,
	}, nil
}

func (o *ClusterUninstaller) GetStages() []providers.Stage {
	return []providers.Stage{
		{
			Name:  "Stop virtual machines",
			Funcs: []providers.StageFunc{o.stopVirtualMachines},
		}, {
			Name:  "Destroy virtual machines",
			Funcs: []providers.StageFunc{o.deleteVirtualMachines},
		}, {
			Name:  "Destroy Folder",
			Funcs: []providers.StageFunc{o.deleteFolder},
		}, {
			Name:  "Delete Storage Policy and Tags",
			Funcs: []providers.StageFunc{o.deleteStoragePolicy, o.deleteTag, o.deleteTagCategory},
		}, {
			Name:  "Cleanup clients",
			Funcs: []providers.StageFunc{o.cleanupClients},
		},
	}
}

func isNotFound(err error) bool {
	return err != nil && strings.HasSuffix(err.Error(), http.StatusText(http.StatusNotFound))
}

func (o *ClusterUninstaller) cleanupClients(_ context.Context) (bool, error) {
	o.clientCleanup()
	return true, nil
}

func (o *ClusterUninstaller) contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(o.context, defaultTimeout)
}

func (o *ClusterUninstaller) getAttachedObjectsOnTag(objType string) ([]types.ManagedObjectReference, error) {
	ctx, cancel := o.contextWithTimeout()
	defer cancel()

	o.Logger.Debugf("Find attached %s on tag", objType)
	tagManager := tags.NewManager(o.RestClient)
	attached, err := tagManager.GetAttachedObjectsOnTags(ctx, []string{o.InfraID})
	if err != nil && !isNotFound(err) {
		return nil, err
	}

	// Separate the objects attached to the tag based on type
	var objectList []types.ManagedObjectReference
	for _, attachedObject := range attached {
		for _, ref := range attachedObject.ObjectIDs {
			if ref.Reference().Type == objType {
				objectList = append(objectList, ref.Reference())
			}
		}
	}

	return objectList, nil
}

func (o *ClusterUninstaller) getFolderManagedObjects(moRef []types.ManagedObjectReference) ([]mo.Folder, error) {
	ctx, cancel := o.contextWithTimeout()
	defer cancel()

	var folderMoList []mo.Folder
	if len(moRef) > 0 {
		pc := property.DefaultCollector(o.Client)
		err := pc.Retrieve(ctx, moRef, nil, &folderMoList)
		if err != nil {
			return nil, err
		}
	}
	return folderMoList, nil
}

func (o *ClusterUninstaller) listFolders() ([]mo.Folder, error) {
	folderList, err := o.getAttachedObjectsOnTag("Folder")
	if err != nil {
		return nil, err
	}

	return o.getFolderManagedObjects(folderList)
}

func (o *ClusterUninstaller) deleteFolder(ctx context.Context) (bool, error) {
	ctx, cancel := o.contextWithTimeout()
	defer cancel()

	o.Logger.Debug("Delete Folder")

	folderMoList, err := o.listFolders()
	if err != nil {
		o.Logger.Debugf("could not list folders: %v", err)
		return false, nil
	}

	// The installer should create at most one parent,
	// the parent to the VirtualMachines.
	// If there are more or less fail with error message.
	if o.terraformPlatform != vspheretypes.ZoningTerraformName {
		if len(folderMoList) > 1 {
			return false, errors.Errorf("Expected 1 Folder per tag but got %d", len(folderMoList))
		}
	}

	if len(folderMoList) == 0 {
		o.Logger.Debug("All folders deleted")
		return true, nil
	}

	// If there are no children in the folder, go ahead and remove it

	for _, f := range folderMoList {
		folderLogger := o.Logger.WithField("Folder", f.Name)
		if numChildren := len(f.ChildEntity); numChildren > 0 {
			entities := make([]string, 0, numChildren)
			for _, child := range f.ChildEntity {
				entities = append(entities, fmt.Sprintf("%s:%s", child.Type, child.Value))
			}
			folderLogger.Errorf("Folder should be empty but contains %d objects: %s. The installer will retry removing \"virtualmachine\" objects, but any other type will need to be removed manually before the deprovision can proceed", numChildren, strings.Join(entities, ", "))
			return false, errors.Errorf("Expected Folder %s to be empty", f.Name)
		}
		folder := object.NewFolder(o.Client, f.Reference())
		task, err := folder.Destroy(ctx)
		if err == nil {
			err = task.Wait(ctx)
		}
		if err != nil {
			folderLogger.Debug(err)
			return false, nil
		}
		folderLogger.Info("Destroyed")
	}

	return true, nil
}

func (o *ClusterUninstaller) deleteStoragePolicy(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*30)
	defer cancel()

	o.Logger.Debug("Delete Storage Policy")
	rtype := pbmtypes.PbmProfileResourceType{
		ResourceType: string(pbmtypes.PbmProfileResourceTypeEnumSTORAGE),
	}

	category := pbmtypes.PbmProfileCategoryEnumREQUIREMENT

	pbmClient, err := pbm.NewClient(ctx, o.Client)
	if err != nil {
		o.Logger.Debugf("could not get pbm client: %v", err)
		return false, nil
	}

	ids, err := pbmClient.QueryProfile(ctx, rtype, string(category))
	if err != nil {
		o.Logger.Debugf("could not query profile: %v", err)
		return false, nil
	}

	profiles, err := pbmClient.RetrieveContent(ctx, ids)
	if err != nil {
		o.Logger.Debug("could not retrieve profile: %v", err)
		return false, nil
	}
	policyName := fmt.Sprintf("openshift-storage-policy-%s", o.InfraID)
	policyLogger := o.Logger.WithField("StoragePolicy", policyName)

	matchingProfileIds := []pbmtypes.PbmProfileId{}
	for _, p := range profiles {
		if p.GetPbmProfile().Name == policyName {
			profileID := p.GetPbmProfile().ProfileId
			matchingProfileIds = append(matchingProfileIds, profileID)
		}
	}
	if len(matchingProfileIds) > 0 {
		_, err = pbmClient.DeleteProfile(ctx, matchingProfileIds)
		if err != nil {
			policyLogger.Debugf("failed to delete profile: %v", err)
			return false, nil
		}
		policyLogger.Info("Destroyed")

	}
	return true, nil
}

func (o *ClusterUninstaller) deleteTag(ctx context.Context) (bool, error) {
	ctx, cancel := o.contextWithTimeout()
	defer cancel()

	tagLogger := o.Logger.WithField("Tag", o.InfraID)
	tagLogger.Debug("Delete")

	tagManager := tags.NewManager(o.RestClient)
	tag, err := tagManager.GetTag(ctx, o.InfraID)
	if err == nil {
		err = tagManager.DeleteTag(ctx, tag)
		if err == nil {
			tagLogger.Info("Deleted")
			return true, nil
		}
	}
	if isNotFound(err) {
		return true, nil
	}
	tagLogger.Debugf("failed to delete tag: %v", err)
	return false, nil
}

func (o *ClusterUninstaller) deleteTagCategory(ctx context.Context) (bool, error) {
	ctx, cancel := o.contextWithTimeout()
	defer cancel()

	categoryID := "openshift-" + o.InfraID
	tcLogger := o.Logger.WithField("TagCategory", categoryID)
	tcLogger.Debug("Delete")

	tagManager := tags.NewManager(o.RestClient)
	ids, err := tagManager.ListCategories(ctx)
	if err != nil {
		tcLogger.Errorln(err)
		return false, nil
	}

	var errs []error
	for _, id := range ids {
		category, err := tagManager.GetCategory(ctx, id)
		if err != nil {
			if !isNotFound(err) {
				errs = append(errs, errors.Wrapf(err, "could not get category %q", id))
			}
			continue
		}
		if category.Name == categoryID {
			if err = tagManager.DeleteCategory(ctx, category); err != nil {
				tcLogger.Errorln(err)
				return false, nil
			}
			tcLogger.Info("Deleted")
			return true, nil
		}
	}

	if len(errs) == 0 {
		tcLogger.Debug("Not found")
		return true, nil
	}
	tcLogger.Debugln(utilerrors.NewAggregate(errs))
	return false, nil
}
