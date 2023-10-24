package search

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/location"

	"github.com/Azure/azure-sdk-for-go/services/search/mgmt/2020-03-13/search"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/search/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tags"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceSearchService() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceSearchServiceCreateUpdate,
		Read:   resourceSearchServiceRead,
		Update: resourceSearchServiceCreateUpdate,
		Delete: resourceSearchServiceDelete,

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.SearchServiceID(id)
			return err
		}),

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:     pluginsdk.TypeString,
				Required: true,
				ForceNew: true,
			},

			"location": azure.SchemaLocation(),

			"resource_group_name": azure.SchemaResourceGroupName(),

			"sku": {
				Type:     pluginsdk.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(search.Free),
					string(search.Basic),
					string(search.Standard),
					string(search.Standard2),
					string(search.Standard3),
					string(search.StorageOptimizedL1),
					string(search.StorageOptimizedL2),
				}, false),
			},

			"replica_count": {
				Type:     pluginsdk.TypeInt,
				Optional: true,
				Computed: true,
			},

			"partition_count": {
				Type:         pluginsdk.TypeInt,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.IntAtMost(12),
			},

			"primary_key": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"secondary_key": {
				Type:     pluginsdk.TypeString,
				Computed: true,
			},

			"query_keys": {
				Type:     pluginsdk.TypeList,
				Computed: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"name": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},

						"key": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
					},
				},
			},

			"public_network_access_enabled": {
				Type:     pluginsdk.TypeBool,
				Optional: true,
				Default:  true,
			},

			"allowed_ips": {
				Type:     pluginsdk.TypeList,
				Optional: true,
				Elem: &pluginsdk.Schema{
					Type: pluginsdk.TypeString,
					ValidateFunc: validation.Any(
						validate.IPv4Address,
						validate.CIDR,
					),
				},
			},

			"identity": {
				Type:     pluginsdk.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"type": {
							Type:     pluginsdk.TypeString,
							Required: true,
							ValidateFunc: validation.StringInSlice([]string{
								string(search.SystemAssigned),
							}, false),
						},

						"principal_id": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},

						"tenant_id": {
							Type:     pluginsdk.TypeString,
							Computed: true,
						},
					},
				},
			},

			"tags": tags.Schema(),
		},
	}
}

func resourceSearchServiceCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Search.ServicesClient
	subscriptionId := meta.(*clients.Client).Account.SubscriptionId
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id := parse.NewSearchServiceID(subscriptionId, d.Get("resource_group_name").(string), d.Get("name").(string))
	if d.IsNewResource() {
		existing, err := client.Get(ctx, id.ResourceGroup, id.Name, nil)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for presence of existing %s: %+v", id, err)
			}
		}

		if !utils.ResponseWasNotFound(existing.Response) {
			return tf.ImportAsExistsError("azurerm_search_service", id.ID())
		}
	}

	location := azure.NormalizeLocation(d.Get("location").(string))
	skuName := d.Get("sku").(string)

	publicNetworkAccess := search.Enabled
	if enabled := d.Get("public_network_access_enabled").(bool); !enabled {
		publicNetworkAccess = search.Disabled
	}

	t := d.Get("tags").(map[string]interface{})

	properties := search.Service{
		Location: utils.String(location),
		Sku: &search.Sku{
			Name: search.SkuName(skuName),
		},
		ServiceProperties: &search.ServiceProperties{
			PublicNetworkAccess: publicNetworkAccess,
			NetworkRuleSet: &search.NetworkRuleSet{
				IPRules: expandSearchServiceIPRules(d.Get("allowed_ips").([]interface{})),
			},
		},
		Identity: expandSearchServiceIdentity(d.Get("identity").([]interface{})),
		Tags:     tags.Expand(t),
	}

	if v, ok := d.GetOk("replica_count"); ok {
		replicaCount := int32(v.(int))
		properties.ServiceProperties.ReplicaCount = utils.Int32(replicaCount)
	}

	if v, ok := d.GetOk("partition_count"); ok {
		partitionCount := int32(v.(int))
		properties.ServiceProperties.PartitionCount = utils.Int32(partitionCount)
	}

	future, err := client.CreateOrUpdate(ctx, id.ResourceGroup, id.Name, properties, nil)
	if err != nil {
		return fmt.Errorf("creating/updating %s: %+v", id, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting for the creation/update of %s: %+v", id, err)
	}

	d.SetId(id.ID())
	return resourceSearchServiceRead(d, meta)
}

func resourceSearchServiceRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Search.ServicesClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.SearchServiceID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, id.ResourceGroup, id.Name, nil)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[DEBUG] %s was not found - removing from state", *id)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("retrieving %s: %+v", *id, err)
	}

	d.Set("name", id.Name)
	d.Set("resource_group_name", id.ResourceGroup)
	d.Set("location", location.NormalizeNilable(resp.Location))

	if sku := resp.Sku; sku != nil {
		d.Set("sku", string(sku.Name))
	}

	if props := resp.ServiceProperties; props != nil {
		if count := props.PartitionCount; count != nil {
			d.Set("partition_count", int(*count))
		}

		if count := props.ReplicaCount; count != nil {
			d.Set("replica_count", int(*count))
		}

		d.Set("public_network_access_enabled", props.PublicNetworkAccess != "Disabled")

		d.Set("allowed_ips", flattenSearchServiceIPRules(props.NetworkRuleSet))
	}

	adminKeysClient := meta.(*clients.Client).Search.AdminKeysClient
	adminKeysResp, err := adminKeysClient.Get(ctx, id.ResourceGroup, id.Name, nil)
	if err == nil {
		d.Set("primary_key", adminKeysResp.PrimaryKey)
		d.Set("secondary_key", adminKeysResp.SecondaryKey)
	}

	queryKeysClient := meta.(*clients.Client).Search.QueryKeysClient
	queryKeysResp, err := queryKeysClient.ListBySearchService(ctx, id.ResourceGroup, id.Name, nil)
	if err == nil {
		d.Set("query_keys", flattenSearchQueryKeys(queryKeysResp.Values()))
	}

	if err := d.Set("identity", flattenSearchServiceIdentity(resp.Identity)); err != nil {
		return fmt.Errorf("setting `identity`: %s", err)
	}

	return tags.FlattenAndSet(d, resp.Tags)
}

func resourceSearchServiceDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Search.ServicesClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.SearchServiceID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Delete(ctx, id.ResourceGroup, id.Name, nil)
	if err != nil {
		if utils.ResponseWasNotFound(resp) {
			return nil
		}

		return fmt.Errorf("deleting %s: %+v", *id, err)
	}

	return nil
}

func flattenSearchQueryKeys(input []search.QueryKey) []interface{} {
	results := make([]interface{}, 0)

	for _, v := range input {
		result := make(map[string]interface{})

		if v.Name != nil {
			result["name"] = *v.Name
		}
		result["key"] = *v.Key

		results = append(results, result)
	}

	return results
}

func expandSearchServiceIPRules(input []interface{}) *[]search.IPRule {
	output := make([]search.IPRule, 0)
	if input == nil {
		return &output
	}

	for _, rule := range input {
		if rule != nil {
			output = append(output, search.IPRule{
				Value: utils.String(rule.(string)),
			})
		}
	}

	return &output
}

func flattenSearchServiceIPRules(input *search.NetworkRuleSet) []interface{} {
	if input == nil || *input.IPRules == nil || len(*input.IPRules) == 0 {
		return nil
	}
	result := make([]interface{}, 0)
	for _, rule := range *input.IPRules {
		result = append(result, rule.Value)
	}
	return result
}

func expandSearchServiceIdentity(input []interface{}) *search.Identity {
	if len(input) == 0 || input[0] == nil {
		return &search.Identity{
			Type: search.None,
		}
	}
	identity := input[0].(map[string]interface{})
	return &search.Identity{
		Type: search.IdentityType(identity["type"].(string)),
	}
}

func flattenSearchServiceIdentity(identity *search.Identity) []interface{} {
	if identity == nil || identity.Type == search.None {
		return make([]interface{}, 0)
	}

	principalId := ""
	if identity.PrincipalID != nil {
		principalId = *identity.PrincipalID
	}

	tenantId := ""
	if identity.TenantID != nil {
		tenantId = *identity.TenantID
	}

	return []interface{}{
		map[string]interface{}{
			"principal_id": principalId,
			"tenant_id":    tenantId,
			"type":         string(identity.Type),
		},
	}
}
