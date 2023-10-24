package consumption

import (
	"strings"

	"github.com/hashicorp/terraform-provider-azurerm/internal/sdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/consumption/migration"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/consumption/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
)

type SubscriptionConsumptionBudget struct {
	base consumptionBudgetBaseResource
}

var _ sdk.Resource = SubscriptionConsumptionBudget{}
var _ sdk.ResourceWithCustomImporter = SubscriptionConsumptionBudget{}
var _ sdk.ResourceWithStateMigration = SubscriptionConsumptionBudget{}

func (r SubscriptionConsumptionBudget) Arguments() map[string]*pluginsdk.Schema {
	schema := map[string]*pluginsdk.Schema{
		"name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: validation.StringIsNotWhiteSpace,
		},
		"subscription_id": {
			Type:     pluginsdk.TypeString,
			Required: true,
			ForceNew: true,
			//ValidateFunc: commonids.ValidateSubscriptionID, // TODO uncomment this in 3.0
			// TODO remove in 3.0
			DiffSuppressFunc: func(k, old, new string, d *pluginsdk.ResourceData) bool {
				n := strings.Split(old, "/")
				if len(n) >= 3 {
					subscriptionID := n[2]
					return new == subscriptionID
				}
				return false
			},
		},
	}
	return r.base.arguments(schema)
}

func (r SubscriptionConsumptionBudget) Attributes() map[string]*pluginsdk.Schema {
	return r.base.attributes()
}

func (r SubscriptionConsumptionBudget) ModelObject() interface{} {
	return nil
}

func (r SubscriptionConsumptionBudget) ResourceType() string {
	return "azurerm_consumption_budget_subscription"
}

func (r SubscriptionConsumptionBudget) IDValidationFunc() pluginsdk.SchemaValidateFunc {
	return validate.ConsumptionBudgetSubscriptionID
}

func (r SubscriptionConsumptionBudget) Create() sdk.ResourceFunc {
	return r.base.createFunc(r.ResourceType(), "subscription_id")
}

func (r SubscriptionConsumptionBudget) Read() sdk.ResourceFunc {
	return r.base.readFunc("subscription_id")
}

func (r SubscriptionConsumptionBudget) Delete() sdk.ResourceFunc {
	return r.base.deleteFunc()
}

func (r SubscriptionConsumptionBudget) Update() sdk.ResourceFunc {
	return r.base.updateFunc()
}

func (r SubscriptionConsumptionBudget) CustomImporter() sdk.ResourceRunFunc {
	return r.base.importerFunc("subscription")
}

func (r SubscriptionConsumptionBudget) StateUpgraders() sdk.StateUpgradeData {
	return sdk.StateUpgradeData{
		SchemaVersion: 1,
		Upgraders: map[int]pluginsdk.StateUpgrade{
			0: migration.SubscriptionConsumptionBudgetV0ToV1{},
		},
	}
}
