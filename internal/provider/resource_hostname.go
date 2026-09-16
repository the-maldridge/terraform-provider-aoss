package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

func aossHostnameResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: aossHostnameWrite,
		ReadContext:   aossHostnameRead,
		UpdateContext: aossHostnameWrite,
		DeleteContext: aossHostnameDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"hostname": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateHostname,
				Description:  "The hostname to configure on the switch.",
			},
		},
	}
}

func validateHostname(v any, k string) ([]string, []error) {
	name, ok := v.(string)
	if !ok {
		return nil, []error{fmt.Errorf("%s must be a string", k)}
	}
	if name == "" {
		return nil, []error{fmt.Errorf("%s must not be empty", k)}
	}
	if len(name) > 63 {
		return nil, []error{fmt.Errorf("%s must be 63 characters or fewer (got %d)", k, len(name))}
	}
	if !hostnameRe.MatchString(name) {
		return nil, []error{fmt.Errorf("%s %q must start and end with an alphanumeric character and contain only letters, digits, dots, underscores, and hyphens", k, name)}
	}
	return nil, nil
}

// openSwitch builds and opens a client from the provider configuration.
// Callers must Close the returned client.
func openSwitch(ctx context.Context, cfg *Config) (*client.Client, error) {
	cl, err := client.New(client.ClientConfig{
		Host:     cfg.Host,
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		return nil, err
	}
	if err := cl.Open(ctx); err != nil {
		_ = cl.Close(ctx)
		return nil, err
	}
	return cl, nil
}

// readHostname fetches the hostname currently set on the switch.
func readHostname(ctx context.Context, cl *client.Client) (string, error) {
	output, err := cl.Show(ctx, "show running-config")
	if err != nil {
		return "", err
	}
	cfg, err := client.ParseRunningConfig(output)
	if err != nil {
		return "", err
	}
	return cfg.Hostname, nil
}

func aossHostnameDelete(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
	// No device-side command: removing the resource only drops it from
	// state. The switch keeps its current hostname.
	return nil
}

func aossHostnameRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	name, err := readHostname(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	if name == "" {
		// The switch has no hostname statement; treat as absent so an
		// import or refresh of a removed resource does not error.
		d.SetId("")
		return nil
	}
	d.SetId(name)
	d.Set("hostname", name)
	return nil
}

func aossHostnameWrite(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	name := d.Get("hostname").(string)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	if _, err := cl.SendConfig(ctx, fmt.Sprintf(`hostname %q`, name)); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(name)
	return nil
}
