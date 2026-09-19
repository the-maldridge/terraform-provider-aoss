package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

func aossPrimaryVLANResource() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages the primary VLAN.",
		CreateContext: aossPrimaryVLANWrite,
		ReadContext:   aossPrimaryVLANRead,
		UpdateContext: aossPrimaryVLANWrite,
		DeleteContext: aossPrimaryVLANDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"vlan_id": {
				Type:         schema.TypeInt,
				Required:     true,
				ValidateFunc: validateVLANID,
				Description:  "The primary VLAN identifier.",
			},
		},
	}
}

// readPrimaryVLAN fetches the primary VLAN id currently set on the
// switch. A return value of 0 means the switch has no primary-vlan
// statement.
func readPrimaryVLAN(ctx context.Context, cl *client.Client) (int, error) {
	output, err := cl.Show(ctx, "show running-config")
	if err != nil {
		return 0, err
	}
	cfg, err := client.ParseRunningConfig(output)
	if err != nil {
		return 0, err
	}
	if cfg.PrimaryVLAN == "" {
		return 0, nil
	}
	return strconv.Atoi(cfg.PrimaryVLAN)
}

func aossPrimaryVLANDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	id, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("resource id %q is not a VLAN id: %w", d.Id(), err))
	}
	cfg := meta.(*Config)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = closeSwitch(ctx, cl)
	}()

	current, err := readPrimaryVLAN(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	if current != id {
		// The primary-vlan statement is already gone from the running
		// config; the delete is idempotent.
		return nil
	}
	if _, err := cl.SendConfig(ctx, fmt.Sprintf("no primary-vlan %d", id)); err != nil {
		return diag.FromErr(err)
	}
	// Verify the statement is actually gone before reporting success.
	after, err := readPrimaryVLAN(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	if after != 0 {
		return diag.FromErr(fmt.Errorf("primary-vlan %d still present after delete", after))
	}
	return nil
}

func aossPrimaryVLANRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = closeSwitch(ctx, cl)
	}()

	id, err := readPrimaryVLAN(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	if id == 0 {
		// The switch has no primary-vlan statement; treat as absent so
		// an import or refresh of a removed resource does not error.
		d.SetId("")
		return nil
	}
	d.SetId(strconv.Itoa(id))
	d.Set("vlan_id", id)
	return nil
}

func aossPrimaryVLANWrite(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	id := d.Get("vlan_id").(int)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = closeSwitch(ctx, cl)
	}()

	if _, err := cl.SendConfig(ctx, fmt.Sprintf("primary-vlan %d", id)); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(strconv.Itoa(id))
	return nil
}
