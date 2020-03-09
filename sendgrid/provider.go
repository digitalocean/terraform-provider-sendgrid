package sendgrid

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

// Config holds provider configuration data
type Config struct {
	APIKey string
}

// Provider returns the Sendgrid Terraform Provider
func Provider() *schema.Provider {
	provider := &schema.Provider{
		Schema: map[string]*schema.Schema{
			"api_key": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("SENDGRID_API_KEY", nil),
				Description: "The API key used for Sendgrid Authorization.",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"sendgrid_subuser": resourceSubuser(),
			"sendgrid_api_key": resourceAPIKey(),
		},
	}

	provider.ConfigureFunc = func(d *schema.ResourceData) (interface{}, error) {
		return &Config{
			APIKey: d.Get("api_key").(string),
		}, nil
	}

	return provider
}

func createTempFile() string {
	tmpfile, err := ioutil.TempFile("", "tf-sg-test")
	if err != nil {
		log.Fatal(err)
	}
	defer tmpfile.Close()

	return tmpfile.Name()
}

func writeFile(fullPath string, data []byte) error {
	dir := filepath.Dir(fullPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0644)
	}

	return ioutil.WriteFile(fullPath, data, 0644)
}
