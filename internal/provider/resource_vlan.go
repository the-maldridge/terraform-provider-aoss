package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

func aossVLANResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: aossVLANWrite,
		ReadContext:   aossVLANRead,
		UpdateContext: aossVLANWrite,
		DeleteContext: aossVLANDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"vlan_id": {
				Type:        schema.TypeInt,
				Required:    true,
				ForceNew:    true,
				Description: "The VLAN identifier.",
			},
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The VLAN name, as shown in the running configuration.",
			},
			"tagged": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        memberSchema(),
				Description: "The tagged members of the VLAN: port numbers and/or trunk circuit names of the form Trk<N>.",
			},
			"untagged": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        memberSchema(),
				Description: "The untagged members of the VLAN: port numbers and/or trunk circuit names of the form Trk<N>.",
			},
			"dhcp": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether the VLAN interface obtains its IP address via DHCP. The switch records the setting as \"ip address dhcp\".",
			},
		},
	}
}

func memberSchema() *schema.Schema {
	return &schema.Schema{
		Type:         schema.TypeString,
		ValidateFunc: validateMember,
	}
}

func validateMember(v any, k string) ([]string, []error) {
	s, ok := v.(string)
	if !ok {
		return nil, []error{fmt.Errorf("%s must be a string", k)}
	}
	if _, err := client.ParseMember(s); err != nil {
		return nil, []error{fmt.Errorf("%s %q must be a port number or a trunk name of the form Trk<N>", k, s)}
	}
	return nil, nil
}

func memberSet(d *schema.ResourceData, key string) []string {
	raw := d.Get(key).(*schema.Set).List()
	refs := make([]string, 0, len(raw))
	for _, v := range raw {
		// The value already passed validateMember, so parsing cannot fail.
		ref, _ := client.ParseMember(v.(string))
		refs = append(refs, ref)
	}
	return refs
}

// readVLANs fetches the effective per-VLAN state from the running
// configuration.
func readVLANs(ctx context.Context, cl *client.Client) (map[int]client.VLANConfig, error) {
	output, err := cl.Show(ctx, "show running-config")
	if err != nil {
		return nil, err
	}
	return client.ParseRunningConfigVLANs(output)
}

func aossVLANRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	vlans, err := readVLANs(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	id, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("resource id %q is not a VLAN id: %w", d.Id(), err))
	}
	vlan, ok := vlans[id]
	if !ok {
		// The VLAN block is gone from the running config; treat as
		// absent so refresh of a removed resource does not error.
		d.SetId("")
		return nil
	}
	d.SetId(strconv.Itoa(id))
	d.Set("vlan_id", id)
	d.Set("name", vlan.Name)
	d.Set("dhcp", vlan.DHCP)
	if err := d.Set("tagged", vlan.Tagged); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("untagged", vlan.Untagged); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func aossVLANWrite(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	id := d.Get("vlan_id").(int)
	isNew := d.IsNewResource()
	d.SetId(strconv.Itoa(id))

	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	var current *client.VLANConfig
	if !isNew {
		vlans, err := readVLANs(ctx, cl)
		if err != nil {
			return diag.FromErr(err)
		}
		if vlan, ok := vlans[id]; ok {
			current = &vlan
		}
	}
	desired := &client.VLANConfig{
		ID:       id,
		Name:     d.Get("name").(string),
		Tagged:   memberSet(d, "tagged"),
		Untagged: memberSet(d, "untagged"),
		DHCP:     d.Get("dhcp").(bool),
	}
	block := client.BuildVLANConfig(current, desired)
	if block == "" {
		return nil
	}
	if _, err := cl.SendConfig(ctx, block); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func aossVLANDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	id, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("resource id %q is not a VLAN id: %w", d.Id(), err))
	}
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	vlans, err := readVLANs(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	vlan, ok := vlans[id]
	if !ok {
		// The VLAN is already gone from the running config; the delete is
		// idempotent.
		return nil
	}
	if err := cl.RemoveVLAN(ctx, id, client.NeedsVLANDeleteConfirm(vlan)); err != nil {
		return diag.FromErr(err)
	}
	// The confirmation path can trip a device failure indicator without
	// failing the delete (and a stale read can mask a real failure), so
	// verify the VLAN is actually gone before reporting success.
	after, err := readVLANs(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	if _, still := after[id]; still {
		return diag.FromErr(fmt.Errorf("aoss_vlan %d still present after delete", id))
	}
	return nil
}
