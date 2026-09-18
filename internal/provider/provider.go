package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Config holds the validated provider configuration, shared with resources
// and data sources via schema.ResourceData.
type Config struct {
	Host     string
	Username string
	Password string
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		return &schema.Provider{
			Schema: map[string]*schema.Schema{
				"host": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "The hostname or IP address of the switch to configure.",
					DefaultFunc: schema.EnvDefaultFunc("AOSS_HOST", nil),
				},
				"username": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "The username used to authenticate to the switch.",
					DefaultFunc: schema.EnvDefaultFunc("AOSS_USERNAME", nil),
				},
				"password": {
					Type:        schema.TypeString,
					Required:    true,
					Sensitive:   true,
					Description: "The password used to authenticate to the switch.",
					DefaultFunc: schema.EnvDefaultFunc("AOSS_PASSWORD", nil),
				},
			},
			ResourcesMap: map[string]*schema.Resource{
				"aoss_hostname":        aossHostnameResource(),
				"aoss_management_vlan": aossManagementVLANResource(),
				"aoss_primary_vlan":    aossPrimaryVLANResource(),
				"aoss_trunk":           aossTrunkResource(),
				"aoss_vlan":            aossVLANResource(),
			},
			DataSourcesMap: map[string]*schema.Resource{
				"aoss_version": aossVersionDataSource(),
			},
			ConfigureContextFunc: providerConfigure,
		}
	}
}

func providerConfigure(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
	cfg := &Config{
		Host:     d.Get("host").(string),
		Username: d.Get("username").(string),
		Password: d.Get("password").(string),
	}
	if err := cfg.validate(); err != nil {
		return nil, diag.FromErr(err)
	}
	return cfg, nil
}

func (c *Config) validate() error {
	switch {
	case c.Host == "":
		return fmt.Errorf("host must be set in the provider configuration or via the AOSS_HOST environment variable")
	case c.Username == "":
		return fmt.Errorf("username must be set in the provider configuration or via the AOSS_USERNAME environment variable")
	case c.Password == "":
		return fmt.Errorf("password must be set in the provider configuration or via the AOSS_PASSWORD environment variable")
	}
	return nil
}
