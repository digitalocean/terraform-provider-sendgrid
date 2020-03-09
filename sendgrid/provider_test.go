package sendgrid

import (
	"encoding/json"
	"log"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

// How to run the acceptance tests for this provider:
//
// - Obtain a Sendgrid Pro account (for the creation of subusers
//   and dedicated IP).
//
// - Set the following environment variables:
//   SENDGRID_API_KEY=<your-api-key>
//   SENDGRID_TEST_IPS=<dedicated-ips>
//
//   where <dedicated-ips> is a JSON array of IP addresses to use.
//   E.g., SENDGRID_TEST_IPS='["127.0.0.1"]'
//
// - Run the Terraform acceptance tests as usual:
//       make testacc

func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

var (
	testProvider  *schema.Provider
	testProviders map[string]terraform.ResourceProvider
	testIPs       []string
	testIPsRaw    string
)

func init() {
	testProvider = Provider()
	testProviders = map[string]terraform.ResourceProvider{
		"sendgrid": testProvider,
	}

	testIPsRaw = os.Getenv("SENDGRID_TEST_IPS")
	if testIPsRaw == "" {
		log.Fatal("SENDGRID_TEST_IPS must be set for acceptance tests")
	}

	err := json.Unmarshal([]byte(testIPsRaw), &testIPs)
	if err != nil {
		log.Fatalf("SENDGRID_TEST_IPS must be a valid JSON string array: %s", err)
	}
}

func testAccPreCheck(t *testing.T) {
	if v := os.Getenv("SENDGRID_API_KEY"); v == "" {
		t.Fatal("SENDGRID_API_KEY must be set for acceptance tests")
	}

	if len(testIPs) < 1 {
		t.Fatal("SENDGRID_TEST_IPS must contain at least one IP for acceptance tests")
	}
}
