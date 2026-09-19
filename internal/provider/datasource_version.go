package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

func aossVersionDataSource() *schema.Resource {
	return &schema.Resource{
		Description: "Provides information about the software version reported by the switch.",
		ReadContext: aossVersionRead,
		Schema: map[string]*schema.Schema{
			"version": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The software version string reported by the switch.",
			},
			"build_date": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The build date stamp of the running software image.",
			},
			"rom_version": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The boot ROM version reported by the switch.",
			},
			"boot_image": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The name of the boot image in use (Primary or Secondary).",
			},
		},
	}
}

func aossVersionRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = closeSwitch(ctx, cl)
	}()

	output, err := cl.Show(ctx, "show version")
	if err != nil {
		return diag.FromErr(err)
	}
	ver, err := client.ParseVersion(output)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(ver.Version)
	d.Set("version", ver.Version)
	d.Set("build_date", ver.BuildDate)
	d.Set("rom_version", ver.RomVersion)
	d.Set("boot_image", ver.BootImage)
	return nil
}
