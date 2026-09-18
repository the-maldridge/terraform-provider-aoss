package provider

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

var trunkNameRe = regexp.MustCompile(`^trk\d+$`)

func aossTrunkResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: aossTrunkWrite,
		ReadContext:   aossTrunkRead,
		UpdateContext: aossTrunkWrite,
		DeleteContext: aossTrunkDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"ports": {
				Type:        schema.TypeSet,
				Required:    true,
				MinItems:    1,
				Elem:        trunkPortSchema(),
				Description: "The switch ports that make up the trunk: a set of port numbers.",
			},
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateTrunkName,
				Description:  "The trunk name; must be of the form trk<N>.",
			},
			"mode": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "lacp",
				ValidateFunc: validateTrunkMode,
				Description:  "The trunk mode: \"trunk\" or \"lacp\" (default).",
			},
		},
	}
}

func trunkPortSchema() *schema.Schema {
	return &schema.Schema{
		Type:         schema.TypeInt,
		ValidateFunc: validateTrunkPort,
	}
}

func validateTrunkPort(v any, k string) ([]string, []error) {
	n, ok := v.(int)
	if !ok {
		return nil, []error{fmt.Errorf("%s must be an integer", k)}
	}
	if n < 1 || n > client.MaxPort {
		return nil, []error{fmt.Errorf("%s must be a port number between 1 and %d (got %d)", k, client.MaxPort, n)}
	}
	return nil, nil
}

func validateTrunkName(v any, k string) ([]string, []error) {
	name, ok := v.(string)
	if !ok {
		return nil, []error{fmt.Errorf("%s must be a string", k)}
	}
	if !trunkNameRe.MatchString(name) {
		return nil, []error{fmt.Errorf("%s %q must be of the form trk<N>", k, name)}
	}
	return nil, nil
}

func validateTrunkMode(v any, k string) ([]string, []error) {
	mode, ok := v.(string)
	if !ok {
		return nil, []error{fmt.Errorf("%s must be a string", k)}
	}
	if mode != "trunk" && mode != "lacp" {
		return nil, []error{fmt.Errorf("%s must be \"trunk\" or \"lacp\" (got %q)", k, mode)}
	}
	return nil, nil
}

// readTrunks fetches the effective per-trunk state from the running
// configuration.
func readTrunks(ctx context.Context, cl *client.Client) (map[string]client.TrunkConfig, error) {
	output, err := cl.Show(ctx, "show running-config")
	if err != nil {
		return nil, err
	}
	return client.ParseRunningConfigTrunks(output)
}

// trunkPortSet returns the set of ports as canonical, numerically
// sorted port references.
func trunkPortSet(d *schema.ResourceData) []string {
	raw := d.Get("ports").(*schema.Set).List()
	nums := make([]int, 0, len(raw))
	for _, v := range raw {
		// The value already passed validateTrunkPort.
		nums = append(nums, v.(int))
	}
	sort.Ints(nums)
	ports := make([]string, 0, len(nums))
	for _, n := range nums {
		// The value already passed validateTrunkPort, so formatting
		// cannot fail.
		ports = append(ports, strconv.Itoa(n))
	}
	return ports
}

// trunkReadPorts converts the parser's canonical port references to the
// ints the `ports` set schema expects.
func trunkReadPorts(ports []string) ([]int, error) {
	nums := make([]int, 0, len(ports))
	for _, p := range ports {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("aoss: invalid port %q in trunk: %w", p, err)
		}
		nums = append(nums, n)
	}
	return nums, nil
}

func aossTrunkRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	trunks, err := readTrunks(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	trunk, ok := trunks[d.Id()]
	if !ok {
		// The trunk line is gone from the running config; treat as
		// absent so refresh of a removed resource does not error.
		d.SetId("")
		return nil
	}
	d.Set("name", trunk.Name)
	d.Set("mode", trunk.Mode)
	nums, err := trunkReadPorts(trunk.Ports)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("ports", nums); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func aossTrunkWrite(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	name := d.Get("name").(string)
	mode := d.Get("mode").(string)
	isNew := d.IsNewResource()
	d.SetId(name)

	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	var current *client.TrunkConfig
	if !isNew {
		trunks, err := readTrunks(ctx, cl)
		if err != nil {
			return diag.FromErr(err)
		}
		if t, ok := trunks[name]; ok {
			current = &t
		}
	}
	desired := &client.TrunkConfig{
		Name:  name,
		Ports: trunkPortSet(d),
		Mode:  mode,
	}
	line := client.BuildTrunkConfig(current, desired)
	if line == "" {
		return nil
	}
	if _, err := cl.SendConfig(ctx, line); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func aossTrunkDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	name := d.Id()
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	trunks, err := readTrunks(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	trunk, ok := trunks[name]
	if !ok {
		// The trunk is already gone from the running config; the delete
		// is idempotent.
		return nil
	}
	if _, err := cl.SendConfig(ctx, "no trunk "+client.FormatMemberRange(trunk.Ports)); err != nil {
		return diag.FromErr(err)
	}
	// Verify the trunk is actually gone before reporting success.
	after, err := readTrunks(ctx, cl)
	if err != nil {
		return diag.FromErr(err)
	}
	if _, stillThere := after[name]; stillThere {
		return diag.FromErr(fmt.Errorf("trunk %s still present after delete", name))
	}
	return nil
}
