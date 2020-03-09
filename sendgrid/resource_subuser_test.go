package sendgrid

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

func TestAccResourceSubuser(t *testing.T) {
	username := acctest.RandomWithPrefix("tf-sg-test-subuser")
	passDest := createTempFile()
	defer os.Remove(passDest)

	resource.Test(t, resource.TestCase{
		Providers: testProviders,
		PreCheck:  func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: testResourceSubuserCreateConfig(username, passDest, false),
				Check: resource.ComposeTestCheckFunc(
					testResourceSubuserCheckSendgrid("sendgrid_subuser.test"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "username", username),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "email", username+"@example.org"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "password.#", "1"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "password.0.destination", passDest),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "password.0.length", "16"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "disabled", "false"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "ips.#", "1"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "domain", "0"),
				),
				PreventDiskCleanup: true,
			},
			{
				Config: testResourceSubuserCreateConfig(username, passDest, true),
				Check: resource.ComposeTestCheckFunc(
					testResourceSubuserCheckSendgrid("sendgrid_subuser.test"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "username", username),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "email", username+"@example.org"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "password.#", "1"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "password.0.destination", passDest),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "password.0.length", "16"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "disabled", "true"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "ips.#", "1"),
					resource.TestCheckResourceAttr("sendgrid_subuser.test", "domain", "0"),
				),
				PreventDiskCleanup: true,
			},
			{
				ResourceName:      "sendgrid_subuser.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     fmt.Sprintf("%s:%s:16", username, passDest),
			},
		},
	})
}

func testResourceSubuserCreateConfig(username, passwordDestination string, disabled bool) string {
	return fmt.Sprintf(`
resource "sendgrid_subuser" "test" {
	username = "%[1]s"
	email    = "%[1]s@example.org"
	password {
		destination = "%[2]s"
	}

	disabled = %[3]t

	ips = %[4]s
}`, username, passwordDestination, disabled, testIPsRaw)
}

func testResourceSubuserCheckSendgrid(resourceName string) resource.TestCheckFunc {
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

		if id != instanceState.Attributes[keyUsername] {
			return fmt.Errorf("id doesn't match username")
		}

		apiKey := testProvider.Meta().(*Config).APIKey
		user, err := getSubuser(apiKey, id)
		if err != nil {
			return fmt.Errorf("error reading user: %w", err)
		}

		if user == nil {
			return fmt.Errorf("user not found")
		}

		if user.Username != instanceState.Attributes[keyUsername] {
			return fmt.Errorf("user.Username does not match")
		}

		if user.Email != instanceState.Attributes[keyEmail] {
			return fmt.Errorf("user.Email does not match")
		}

		if fmt.Sprintf("%t", user.Disabled) != instanceState.Attributes[keyDisabled] {
			return fmt.Errorf("user.Disabled does not match")
		}

		return nil
	}
}
