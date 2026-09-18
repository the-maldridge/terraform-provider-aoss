package provider

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

func aossInterfacesDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: aossInterfacesRead,
		Schema: map[string]*schema.Schema{
			"interfaces": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "The switch interfaces: every physical port and every trunk circuit.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The interface name: the configured port name (empty when unconfigured) for physical ports, the trunk name (\"trk<N>\") for logical ones.",
						},
						"number": {
							Type:        schema.TypeInt,
							Computed:    true,
							Description: "The interface number: the port number for physical ports, 0 for logical (trunk) interfaces.",
						},
						"type": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "The interface type: \"physical\" for standalone ports, \"trunk-member\" for ports enslaved to a trunk, \"logical\" for trunk circuits.",
						},
						"shutdown": {
							Type:        schema.TypeBool,
							Computed:    true,
							Description: "Whether the port is administratively shut down (the inverse of the port-enabled state); always false for logical interfaces.",
						},
						"running": {
							Type:        schema.TypeBool,
							Computed:    true,
							Description: "Whether the link is up (the link status); for logical interfaces, true when at least one member port is running.",
						},
					},
				},
			},
		},
	}
}

func aossInterfacesRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	cfg := meta.(*Config)
	cl, err := openSwitch(ctx, cfg)
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		_ = closeSwitch(ctx, cl)
	}()

	ifaceOut, err := cl.Show(ctx, "show interfaces")
	if err != nil {
		return diag.FromErr(err)
	}
	ports, err := client.ParseInterfaces(ifaceOut)
	if err != nil {
		return diag.FromErr(err)
	}

	cfgOut, err := cl.Show(ctx, "show running-config")
	if err != nil {
		return diag.FromErr(err)
	}
	names, err := client.ParseRunningConfigInterfaces(cfgOut)
	if err != nil {
		return diag.FromErr(err)
	}
	trunks, err := client.ParseRunningConfigTrunks(cfgOut)
	if err != nil {
		return diag.FromErr(err)
	}

	states := make(map[int]interfaceState, len(ports))
	for _, p := range ports {
		out, err := cl.Show(ctx, "show interface "+p.Port)
		if err != nil {
			return diag.FromErr(fmt.Errorf("aoss: show interface %s: %w", p.Port, err))
		}
		detail, err := client.ParseInterfaceDetail(out)
		if err != nil {
			return diag.FromErr(err)
		}
		st := interfaceState{running: detail.LinkStatus == "Up"}
		if strings.EqualFold(detail.PortEnabled, "No") {
			st.shutdown = true
		}
		if n, convErr := strconv.Atoi(p.Port); convErr == nil {
			states[n] = st
		}
	}

	list := make([]map[string]any, 0, len(ports)+len(trunks))
	for _, p := range ports {
		n, err := strconv.Atoi(p.Port)
		if err != nil {
			return diag.FromErr(fmt.Errorf("aoss: invalid port %q in interface list: %w", p.Port, err))
		}
		typ := "physical"
		if p.Trunk != "" {
			typ = "trunk-member"
		}
		st := states[n]
		list = append(list, map[string]any{
			"name":     names[n].Name,
			"number":   n,
			"type":     typ,
			"shutdown": st.shutdown,
			"running":  st.running,
		})
	}
	for _, trunk := range trunks {
		running := false
		for _, ref := range trunk.Ports {
			n, err := strconv.Atoi(ref)
			if err != nil {
				continue
			}
			if st, ok := states[n]; ok && st.running {
				running = true
				break
			}
		}
		list = append(list, map[string]any{
			"name":     trunk.Name,
			"number":   0,
			"type":     "logical",
			"shutdown": false,
			"running":  running,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		ti, tj := list[i]["type"].(string), list[j]["type"].(string)
		if ti != tj {
			if ti == "physical" {
				return true
			}
			if tj == "physical" {
				return false
			}
			if ti == "trunk-member" {
				return true
			}
			return false
		}
		ni, nj := list[i]["number"].(int), list[j]["number"].(int)
		if ni != nj {
			return ni < nj
		}
		return list[i]["name"].(string) < list[j]["name"].(string)
	})

	d.SetId(fmt.Sprintf("interfaces-%d", len(list)))
	if err := d.Set("interfaces", list); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

// interfaceState holds the administrative and link state of one physical
// port, read from its "show interface <port>" detail output.
type interfaceState struct {
	shutdown bool
	running  bool
}
