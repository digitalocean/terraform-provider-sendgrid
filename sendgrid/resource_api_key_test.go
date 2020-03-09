package sendgrid

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

func TestAccResourceAPIKey(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-sg-test-apikey")
	dest := createTempFile()
	defer os.Remove(dest)

	scopes := []string{
		"api_keys.create",
		"api_keys.delete",
		"api_keys.read",
		"api_keys.update",
	}

	resource.Test(t, resource.TestCase{
		Providers: testProviders,
		PreCheck:  func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: testResourceAPIKeyCreateConfig(t, name, dest, scopes, ""),
				Check: resource.ComposeTestCheckFunc(
					testResourceAPIKeyCheckSendgrid(t, "sendgrid_api_key.test"),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "name", name),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "destination", dest),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "scopes.#", strconv.FormatInt(int64(len(scopes)), 10)),
				),
				PreventDiskCleanup: true,
			},
			{
				Config: testResourceAPIKeyCreateConfig(t, name+"updated", dest, []string{"api_keys.read"}, ""),
				Check: resource.ComposeTestCheckFunc(
					testResourceAPIKeyCheckSendgrid(t, "sendgrid_api_key.test"),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "name", name+"updated"),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "destination", dest),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "scopes.#", "1"),
				),
				PreventDiskCleanup: true,
			},
			{
				Config: testResourceSubuserCreateConfig(name+"-user", dest, false) +
					testResourceAPIKeyCreateConfig(t, name, dest, scopes, "sendgrid_subuser.test.id"),
				Check: resource.ComposeTestCheckFunc(
					testResourceAPIKeyCheckSendgrid(t, "sendgrid_api_key.test"),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "name", name),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "destination", dest),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "scopes.#", strconv.FormatInt(int64(len(scopes)), 10)),
					resource.TestCheckResourceAttr("sendgrid_api_key.test", "on_behalf_of", name+"-user"),
				),
				PreventDiskCleanup: true,
			},
			{
				ResourceName:      "sendgrid_api_key.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					resourceState := s.Modules[0].Resources["sendgrid_api_key.test"]
					if resourceState == nil {
						return "", fmt.Errorf("resource not found in state")
					}

					instanceState := resourceState.Primary
					if instanceState == nil {
						return "", fmt.Errorf("resource has no primary instance")
					}

					return fmt.Sprintf("%s:%s:%s", instanceState.ID, dest, name+"-user"), nil
				},
			},
		},
	})
}

func testResourceAPIKeyCreateConfig(t *testing.T, name, dest string, scopes []string, onBehalfOf string) string {
	scopesBytes, err := json.Marshal(scopes)
	if err != nil {
		t.Fatal(err)
	}

	if onBehalfOf == "" {
		onBehalfOf = "\"\""
	}

	return fmt.Sprintf(`
resource "sendgrid_api_key" "test" {
	name = "%s"
	destination = "%s"
	scopes = %s
	on_behalf_of = %s
}`, name, dest, string(scopesBytes), onBehalfOf)
}

func testResourceAPIKeyCheckSendgrid(t *testing.T, resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState := s.Modules[0].Resources[resourceName]
		if resourceState == nil {
			return fmt.Errorf("resource not found in state")
		}

		instanceState := resourceState.Primary
		if instanceState == nil {
			return fmt.Errorf("resource has no primary instance")
		}

		id := instanceState.ID
		onBehalfOf := instanceState.Attributes[keyOnBehalfOf]

		apiKey := testProvider.Meta().(*Config).APIKey
		key, err := getAPIKey(apiKey, id, onBehalfOf)
		if err != nil {
			return fmt.Errorf("error reading API key: %w", err)
		}

		if key == nil {
			return fmt.Errorf("API key not found")
		}

		if key.Name != instanceState.Attributes[keyName] {
			return fmt.Errorf("key.Name does not match")
		}

		scopesLen, err := strconv.ParseInt(instanceState.Attributes["scopes.#"], 10, 64)
		if err != nil {
			return err
		}

		if scopesLen != int64(len(key.Scopes)) {
			return fmt.Errorf("key.Scopes length does not match state")
		}

		for i := 0; i < len(key.Scopes); i++ {
			var found bool
			for stateKey, stateValue := range instanceState.Attributes {
				if strings.HasPrefix(stateKey, keyScopes) {
					if key.Scopes[i] == stateValue {
						found = true
						break
					}
				}
			}

			if !found {
				return fmt.Errorf("Scope '%s' missing from state", key.Scopes[i])
			}
		}

		return nil
	}
}
