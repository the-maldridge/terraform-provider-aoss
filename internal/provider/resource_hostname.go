package provider

import (
	"context"
	"fmt"
	"regexp"
	"sync"

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

// switchMu serializes all switch sessions. The device accepts only a
// small number of concurrent SSH sessions, and terraform runs resource
// operations in parallel, so at most one session is open at a time: the
// lock is taken in openSwitch and released in closeSwitch.
var switchMu sync.Mutex

// openSwitch builds and opens a client from the provider configuration.
// Callers must release the session with closeSwitch.
func openSwitch(ctx context.Context, cfg *Config) (*client.Client, error) {
	switchMu.Lock()
	cl, err := client.New(client.ClientConfig{
		Host:     cfg.Host,
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		switchMu.Unlock()
		return nil, err
	}
	if err := cl.Open(ctx); err != nil {
		_ = cl.Close(ctx)
		switchMu.Unlock()
		return nil, err
	}
	return cl, nil
}

// closeSwitch closes the session and releases switchMu.
func closeSwitch(ctx context.Context, cl *client.Client) error {
	err := cl.Close(ctx)
	switchMu.Unlock()
	return err
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
		_ = closeSwitch(ctx, cl)
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
		_ = closeSwitch(ctx, cl)
	}()

	if _, err := cl.SendConfig(ctx, fmt.Sprintf(`hostname %q`, name)); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(name)
	return nil
}
