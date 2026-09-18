package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

func aossInterfaceResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: aossInterfaceWrite,
		ReadContext:   aossInterfaceRead,
		UpdateContext: aossInterfaceWrite,
		DeleteContext: aossInterfaceDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"interface": {
				Type:         schema.TypeInt,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateInterfaceNumber,
				Description:  "The interface (port) number, between 1 and 255.",
			},
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The name to assign to the port; empty for the default (no configured name).",
			},
			"shutdown": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether the port is administratively shut down.",
			},
		},
	}
}

func validateInterfaceNumber(v any, k string) ([]string, []error) {
	n, ok := v.(int)
	if !ok {
		return nil, []error{fmt.Errorf("%s must be an integer", k)}
	}
	if n < 1 || n > client.MaxPort {
		return nil, []error{fmt.Errorf("%s must be a port number between 1 and %d (got %d)", k, client.MaxPort, n)}
	}
	return nil, nil
}

// readInterfaces fetches the effective per-port configuration from the
// running configuration.
func readInterfaces(ctx context.Context, cl *client.Client) (map[int]client.InterfaceConfig, error) {
	output, err := cl.Show(ctx, "show running-config")
	if err != nil {
		return nil, err
	}
	return client.ParseRunningConfigInterfaces(output)
}

// portExists reports whether port is a physical port present on this
// chassis, per the "show interfaces" summary.
func portExists(ctx context.Context, cl *client.Client, port int) (bool, error) {
	output, err := cl.Show(ctx, "show interfaces")
	if err != nil {
		return false, err
	}
	ports, err := client.ParseInterfaces(output)
	if err != nil {
		return false, err
	}
	want := strconv.Itoa(port)
	for _, p := range ports {
		if p.Port == want {
			return true, nil
		}
	}
	return false, nil
}

func aossInterfaceRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	id, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("resource id %q is not a port number: %w", d.Id(), err))
	}

	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = closeSwitch(ctx, cl)
	}()

	exists, err := portExists(ctx, cl, id)
	if err != nil {
		return diag.FromErr(err)
	}
	if !exists {
		// The port is not present on this chassis; treat as absent so
		// refresh or import of a removed port does not error.
		d.SetId("")
		return nil
	}

	ifaces, err := readInterfaces(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	iface := ifaces[id]
	d.SetId(strconv.Itoa(id))
	d.Set("interface", id)
	d.Set("name", iface.Name)
	d.Set("shutdown", iface.Shutdown)
	return nil
}

func aossInterfaceWrite(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	id := d.Get("interface").(int)
	isNew := d.IsNewResource()
	d.SetId(strconv.Itoa(id))

	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = closeSwitch(ctx, cl)
	}()

	var current *client.InterfaceConfig
	if !isNew {
		ifaces, err := readInterfaces(ctx, cl)
		if err != nil {
			return diag.FromErr(err)
		}
		if iface, ok := ifaces[id]; ok {
			current = &iface
		}
	}
	desired := &client.InterfaceConfig{
		Port:     id,
		Name:     d.Get("name").(string),
		Shutdown: d.Get("shutdown").(bool),
	}
	block := client.BuildInterfaceConfig(current, desired)
	if block == "" {
		return nil
	}
	if _, err := cl.SendConfig(ctx, block); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

// buildInterfaceDeleteBlock returns the configuration that clears the
// port's name and shutdown state, removing its `interface <n>` block from
// the running configuration. It returns "" when there is nothing to clear.
func buildInterfaceDeleteBlock(port int, current *client.InterfaceConfig) string {
	var lines []string
	if current.Name != "" {
		lines = append(lines, "no name")
	}
	if current.Shutdown {
		lines = append(lines, "enable")
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("interface %d\n%s\nexit", port, strings.Join(lines, "\n"))
}

func aossInterfaceDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	id, err := strconv.Atoi(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("resource id %q is not a port number: %w", d.Id(), err))
	}
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = closeSwitch(ctx, cl)
	}()

	ifaces, err := readInterfaces(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	iface, ok := ifaces[id]
	if !ok {
		// No interface configuration to remove; the delete is idempotent.
		return nil
	}
	block := buildInterfaceDeleteBlock(id, &iface)
	if block == "" {
		return nil
	}
	if _, err := cl.SendConfig(ctx, block); err != nil {
		return diag.FromErr(err)
	}
	// Verify the configuration is actually gone before reporting success.
	after, err := readInterfaces(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	if _, stillThere := after[id]; stillThere {
		return diag.FromErr(fmt.Errorf("interface %d still has configuration after delete", id))
	}
	return nil
}
