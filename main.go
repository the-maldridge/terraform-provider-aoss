package main

import (
	"flag"

	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"

	"github.com/the-maldridge/terraform-provider-aoss/internal/provider"
)

var version string = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: provider.New(version),
		ProviderAddr: "registry.terraform.io/the-maldridge/aoss",
		Debug:        debug,
	})
}
