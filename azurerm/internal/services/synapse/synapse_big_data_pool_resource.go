package synapse

import (
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/preview/synapse/mgmt/2019-06-01-preview/synapse"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/clients"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/services/synapse/parse"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/services/synapse/validate"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tags"
	azSchema "github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/tf/schema"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/internal/timeouts"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmSynapseBigDataPool() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmSynapseBigDataPoolCreateUpdate,
		Read:   resourceArmSynapseBigDataPoolRead,
		Update: resourceArmSynapseBigDataPoolCreateUpdate,
		Delete: resourceArmSynapseBigDataPoolDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},

		Importer: azSchema.ValidateResourceIDPriorToImport(func(id string) error {
			_, err := parse.SynapseBigDataPoolID(id)
			return err
		}),

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.SynapseBigDataPoolName,
			},

			"synapse_workspace_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.SynapseWorkspaceID,
			},

			"node_size_family": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(synapse.NodeSizeFamilyMemoryOptimized),
				}, false),
			},

			"node_size": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(synapse.NodeSizeSmall),
					string(synapse.NodeSizeMedium),
					string(synapse.NodeSizeLarge),
				}, false),
			},

			"node_count": {
				Type:         schema.TypeInt,
				Optional:     true,
				ValidateFunc: validation.IntBetween(3, 200),
				ExactlyOneOf: []string{"node_count", "auto_scale"},
			},

			"auto_scale": {
				Type:         schema.TypeList,
				Optional:     true,
				MaxItems:     1,
				ExactlyOneOf: []string{"node_count", "auto_scale"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"min_node_count": {
							Type:         schema.TypeInt,
							Required:     true,
							ValidateFunc: validation.IntBetween(3, 200),
						},

						"max_node_count": {
							Type:         schema.TypeInt,
							Required:     true,
							ValidateFunc: validation.IntBetween(3, 200),
						},
					},
				},
			},

			"auto_pause": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"delay_in_minutes": {
							Type:         schema.TypeInt,
							Required:     true,
							ValidateFunc: validation.IntBetween(5, 10080),
						},
					},
				},
			},

			"spark_events_folder": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "/events",
			},

			"spark_log_folder": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "/logs",
			},

			"library_requirement": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"content": {
							Type:     schema.TypeString,
							Required: true,
						},

						"filename": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},

			"spark_version": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "2.4",
				ValidateFunc: validation.StringInSlice([]string{
					"2.4",
				}, false),
			},

			"tags": tags.Schema(),
		},
	}
}

func resourceArmSynapseBigDataPoolCreateUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Synapse.BigDataPoolClient
	workspaceClient := meta.(*clients.Client).Synapse.WorkspaceClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	name := d.Get("name").(string)
	workspaceId, _ := parse.SynapseWorkspaceID(d.Get("synapse_workspace_id").(string))

	if d.IsNewResource() {
		existing, err := client.Get(ctx, workspaceId.ResourceGroup, workspaceId.Name, name)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for present of existing Synapse BigDataPool %q (Resource Group %q / workspaceName %q): %+v", name, workspaceId.ResourceGroup, workspaceId.Name, err)
			}
		}
		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_synapse_big_data_pool", *existing.ID)
		}
	}

	workspace, err := workspaceClient.Get(ctx, workspaceId.ResourceGroup, workspaceId.Name)
	if err != nil {
		return fmt.Errorf("reading Synapse workspace %q (Resource Group %q): %+v", workspaceId.Name, workspaceId.ResourceGroup, err)
	}

	autoScale := expandArmBigDataPoolAutoScaleProperties(d.Get("auto_scale").([]interface{}))
	bigDataPoolInfo := synapse.BigDataPoolResourceInfo{
		Location: workspace.Location,
		BigDataPoolResourceProperties: &synapse.BigDataPoolResourceProperties{
			AutoPause:             expandArmBigDataPoolAutoPauseProperties(d.Get("auto_pause").([]interface{})),
			AutoScale:             autoScale,
			DefaultSparkLogFolder: utils.String(d.Get("spark_log_folder").(string)),
			LibraryRequirements:   expandArmBigDataPoolLibraryRequirements(d.Get("library_requirement").([]interface{})),
			NodeSize:              synapse.NodeSize(d.Get("node_size").(string)),
			NodeSizeFamily:        synapse.NodeSizeFamily(d.Get("node_size_family").(string)),
			SparkEventsFolder:     utils.String(d.Get("spark_events_folder").(string)),
			SparkVersion:          utils.String(d.Get("spark_version").(string)),
		},
		Tags: tags.Expand(d.Get("tags").(map[string]interface{})),
	}
	if !*autoScale.Enabled {
		bigDataPoolInfo.NodeCount = utils.Int32(int32(d.Get("node_count").(int)))
	}

	future, err := client.CreateOrUpdate(ctx, workspaceId.ResourceGroup, workspaceId.Name, name, bigDataPoolInfo, nil)
	if err != nil {
		return fmt.Errorf("creating Synapse BigDataPool %q (Resource Group %q / workspaceName %q): %+v", name, workspaceId.ResourceGroup, workspaceId.Name, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting on creating future for Synapse BigDataPool %q (Resource Group %q / workspaceName %q): %+v", name, workspaceId.ResourceGroup, workspaceId.Name, err)
	}

	resp, err := client.Get(ctx, workspaceId.ResourceGroup, workspaceId.Name, name)
	if err != nil {
		return fmt.Errorf("retrieving Synapse BigDataPool %q (Resource Group %q / workspaceName %q): %+v", name, workspaceId.ResourceGroup, workspaceId.Name, err)
	}

	if resp.ID == nil || *resp.ID == "" {
		return fmt.Errorf("empty or nil ID returned for Synapse BigDataPool %q (Resource Group %q / workspaceName %q) ID", name, workspaceId.ResourceGroup, workspaceId.Name)
	}

	d.SetId(*resp.ID)
	return resourceArmSynapseBigDataPoolRead(d, meta)
}

func resourceArmSynapseBigDataPoolRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Synapse.BigDataPoolClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.SynapseBigDataPoolID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, id.Workspace.ResourceGroup, id.Workspace.Name, id.Name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[INFO] Synapse BigDataPool %q does not exist - removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("retrieving Synapse BigDataPool %q (Resource Group %q / workspaceName %q): %+v", id.Name, id.Workspace.ResourceGroup, id.Workspace.Name, err)
	}
	d.Set("name", id.Name)
	d.Set("synapse_workspace_id", id.Workspace.String())
	if props := resp.BigDataPoolResourceProperties; props != nil {
		if err := d.Set("auto_pause", flattenArmBigDataPoolAutoPauseProperties(props.AutoPause)); err != nil {
			return fmt.Errorf("setting `auto_pause`: %+v", err)
		}
		if err := d.Set("auto_scale", flattenArmBigDataPoolAutoScaleProperties(props.AutoScale)); err != nil {
			return fmt.Errorf("setting `auto_scale`: %+v", err)
		}
		if err := d.Set("library_requirement", flattenArmBigDataPoolLibraryRequirements(props.LibraryRequirements)); err != nil {
			return fmt.Errorf("setting `library_requirement`: %+v", err)
		}
		d.Set("node_count", props.NodeCount)
		d.Set("node_size", props.NodeSize)
		d.Set("node_size_family", string(props.NodeSizeFamily))
		d.Set("spark_version", props.SparkVersion)
	}
	return tags.FlattenAndSet(d, resp.Tags)
}

func resourceArmSynapseBigDataPoolDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Synapse.BigDataPoolClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.SynapseBigDataPoolID(d.Id())
	if err != nil {
		return err
	}

	future, err := client.Delete(ctx, id.Workspace.ResourceGroup, id.Workspace.Name, id.Name)
	if err != nil {
		return fmt.Errorf("deleting Synapse BigDataPool %q (Resource Group %q / workspaceName %q): %+v", id.Name, id.Workspace.ResourceGroup, id.Workspace.Name, err)
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return fmt.Errorf("waiting for deleting Synapse BigDataPool %q (Resource Group %q / workspaceName %q): %+v", id.Name, id.Workspace.ResourceGroup, id.Workspace.Name, err)
	}
	return nil
}

func expandArmBigDataPoolAutoPauseProperties(input []interface{}) *synapse.AutoPauseProperties {
	if len(input) == 0 {
		return &synapse.AutoPauseProperties{
			Enabled: utils.Bool(false),
		}
	}
	v := input[0].(map[string]interface{})
	return &synapse.AutoPauseProperties{
		DelayInMinutes: utils.Int32(int32(v["delay_in_minutes"].(int))),
		Enabled:        utils.Bool(true),
	}
}

func expandArmBigDataPoolAutoScaleProperties(input []interface{}) *synapse.AutoScaleProperties {
	if len(input) == 0 || input[0] == nil {
		return &synapse.AutoScaleProperties{
			Enabled: utils.Bool(false),
		}
	}
	v := input[0].(map[string]interface{})
	return &synapse.AutoScaleProperties{
		MinNodeCount: utils.Int32(int32(v["min_node_count"].(int))),
		Enabled:      utils.Bool(true),
		MaxNodeCount: utils.Int32(int32(v["max_node_count"].(int))),
	}
}

func expandArmBigDataPoolLibraryRequirements(input []interface{}) *synapse.LibraryRequirements {
	if len(input) == 0 {
		return nil
	}
	v := input[0].(map[string]interface{})
	return &synapse.LibraryRequirements{
		Content:  utils.String(v["content"].(string)),
		Filename: utils.String(v["filename"].(string)),
	}
}

func flattenArmBigDataPoolAutoPauseProperties(input *synapse.AutoPauseProperties) []interface{} {
	if input == nil {
		return make([]interface{}, 0)
	}

	var delayInMinutes int32
	if input.DelayInMinutes != nil {
		delayInMinutes = *input.DelayInMinutes
	}
	var enabled bool
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	if !enabled {
		return make([]interface{}, 0)
	}

	return []interface{}{
		map[string]interface{}{
			"delay_in_minutes": delayInMinutes,
		},
	}
}

func flattenArmBigDataPoolAutoScaleProperties(input *synapse.AutoScaleProperties) []interface{} {
	if input == nil {
		return make([]interface{}, 0)
	}

	var enabled bool
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	if !enabled {
		return make([]interface{}, 0)
	}

	var maxNodeCount int32
	if input.MaxNodeCount != nil {
		maxNodeCount = *input.MaxNodeCount
	}
	var minNodeCount int32
	if input.MinNodeCount != nil {
		minNodeCount = *input.MinNodeCount
	}
	return []interface{}{
		map[string]interface{}{
			"max_node_count": maxNodeCount,
			"min_node_count": minNodeCount,
		},
	}
}

func flattenArmBigDataPoolLibraryRequirements(input *synapse.LibraryRequirements) []interface{} {
	if input == nil {
		return make([]interface{}, 0)
	}

	var content string
	if input.Content != nil {
		content = *input.Content
	}
	var filename string
	if input.Filename != nil {
		filename = *input.Filename
	}
	return []interface{}{
		map[string]interface{}{
			"content":  content,
			"filename": filename,
		},
	}
}
