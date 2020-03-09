package main

import (
	"github.com/digitalocean/terraform-provider-sendgrid/sendgrid"
	"github.com/hashicorp/terraform-plugin-sdk/plugin"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: func() terraform.ResourceProvider {
			return sendgrid.Provider()
		},
	})
}
