package sendgrid

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/pkg/errors"
	"github.com/sendgrid/sendgrid-go"
)

const (
	keyScopes     = "scopes"
	keyName       = "name"
	keyOnBehalfOf = "on_behalf_of"

	headerOnBehalfOf = "on-behalf-of"
)

var (
	createAPIKeyRate = time.Tick(5 * time.Second)
	deleteAPIKeyRate = time.Tick(5 * time.Second)
)

type apiKey struct {
	Name     string        `json:"name"`
	APIKey   string        `json:"api_key"`
	APIKeyID string        `json:"api_key_id"`
	Scopes   []interface{} `json:"scopes"`
}

func resourceAPIKey() *schema.Resource {
	return &schema.Resource{
		Create: resourceAPIKeyCreate,
		Read:   resourceAPIKeyRead,
		Update: resourceAPIKeyUpdate,
		Delete: resourceAPIKeyDelete,
		Importer: &schema.ResourceImporter{
			State: func(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				realID, dest, onBehalfOf, err := parseAPIKeyImportID(d.Id())
				if err != nil {
					return nil, err
				}

				d.Set(keyDestination, dest)
				d.Set(keyOnBehalfOf, onBehalfOf)
				d.SetId(realID)

				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			keyName: &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			keyScopes: &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				MinItems: 1,
			},
			keyOnBehalfOf: &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "",
				ForceNew: true,
			},
			keyDestination: &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceAPIKeyCreate(d *schema.ResourceData, m interface{}) error {
	payload := map[string]interface{}{
		"name":   d.Get(keyName),
		"scopes": d.Get(keyScopes).(*schema.Set).List(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	config := m.(*Config)
	request := sendgrid.GetRequest(config.APIKey, "/v3/api_keys", sendgridAddress)
	request.Method = http.MethodPost
	request.Body = data

	if onBehalfOf := d.Get(keyOnBehalfOf).(string); onBehalfOf != "" {
		request.Headers[headerOnBehalfOf] = onBehalfOf
	}

	res, err := doRequest(request, withStatus(http.StatusCreated), withRateLimit(createAPIKeyRate))
	if err != nil {
		return errors.Wrap(err, "failed to create API key")
	}

	var key apiKey
	err = json.Unmarshal([]byte(res.Body), &key)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal created API key")
	}

	d.SetId(key.APIKeyID)

	err = writeFile(d.Get(keyDestination).(string), []byte(key.APIKey))
	if err != nil {
		return errors.Wrap(err, "failed to write API key to destination")
	}

	return waitForAPIKey(d, m)
}

func resourceAPIKeyRead(d *schema.ResourceData, m interface{}) error {
	config := m.(*Config)
	key, err := getAPIKey(config.APIKey, d.Id(), d.Get(keyOnBehalfOf).(string))
	if err != nil {
		return errors.Wrap(err, "failed to get API key")
	} else if key == nil {
		d.SetId("")
		return nil
	}

	d.Set(keyName, key.Name)
	d.Set(keyScopes, key.Scopes)

	return nil
}

func resourceAPIKeyUpdate(d *schema.ResourceData, m interface{}) error {
	payload := map[string]interface{}{
		"name":   d.Get(keyName).(string),
		"scopes": d.Get(keyScopes).(*schema.Set).List(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to update API key")
	}

	config := m.(*Config)
	request := sendgrid.GetRequest(config.APIKey, "/v3/api_keys/"+d.Id(), sendgridAddress)
	request.Method = http.MethodPut
	request.Body = data

	if onBehalfOf := d.Get(keyOnBehalfOf).(string); onBehalfOf != "" {
		request.Headers[headerOnBehalfOf] = onBehalfOf
	}

	_, err = doRequest(request, withStatus(http.StatusOK))
	if err != nil {
		return errors.Wrap(err, "failed to update API key")
	}

	return waitForAPIKey(d, m)
}

func resourceAPIKeyDelete(d *schema.ResourceData, m interface{}) error {
	config := m.(*Config)
	request := sendgrid.GetRequest(config.APIKey, "/v3/api_keys/"+d.Id(), sendgridAddress)
	request.Method = http.MethodDelete

	if onBehalfOf := d.Get(keyOnBehalfOf).(string); onBehalfOf != "" {
		request.Headers[headerOnBehalfOf] = onBehalfOf
	}

	res, err := doRequest(request, withStatus(http.StatusNoContent), withRateLimit(deleteAPIKeyRate), withRetry(5))
	if err == nil || res.StatusCode == http.StatusNotFound {
		return nil
	}

	return errors.Wrap(err, "failed to delete API key")
}

func getAPIKey(sendgridAPIKey, id, onBehalfOf string) (*apiKey, error) {
	request := sendgrid.GetRequest(sendgridAPIKey, "/v3/api_keys/"+id, sendgridAddress)
	request.Method = http.MethodGet

	log.Println("[TRACE] GET /v3/api_keys/" + id)

	if onBehalfOf != "" {
		request.Headers[headerOnBehalfOf] = onBehalfOf
	}

	// Sendgrid can return a 200 even if not found; but the response body contains
	// 	{
	//    "errors": [
	//      {
	//        "field": null,
	//        "message": "API Key not found"
	//      }
	//    ]
	//  }
	res, err := doRequest(request, withStatus(http.StatusOK), withStatus(http.StatusNotFound))
	if err != nil {
		return nil, errors.Wrap(err, "failed to query API key")
	}

	if res.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	var k apiKey

	err = json.Unmarshal([]byte(res.Body), &k)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal apiKey query response")
	}

	if k.Name == "" {
		return nil, nil
	}

	return &k, nil
}

func waitForAPIKey(d *schema.ResourceData, m interface{}) error {
	config := m.(*Config)
	name := d.Get(keyName).(string)
	scopes := d.Get(keyScopes).(*schema.Set).List()

	createStateConf := &resource.StateChangeConf{
		Pending:                   []string{statusWaiting},
		Target:                    []string{statusDone},
		Timeout:                   d.Timeout(schema.TimeoutCreate),
		Delay:                     defaultBackoff,
		MinTimeout:                defaultBackoff,
		ContinuousTargetOccurence: 3,
		Refresh: func() (interface{}, string, error) {
			key, err := getAPIKey(config.APIKey, d.Id(), d.Get(keyOnBehalfOf).(string))
			if l, ok := err.(ratelimitError); ok {
				time.Sleep(l.timeout)
				return nil, statusWaiting, nil
			} else if err != nil {
				return nil, "", err
			} else if key == nil {
				return nil, statusWaiting, nil
			} else if key.Name != name {
				return nil, statusWaiting, nil
			} else if !sliceContentsAreEqual(key.Scopes, scopes) {
				return nil, statusWaiting, nil
			}

			return key, statusDone, nil
		},
	}

	_, err := createStateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for key %s (id: %s) to be created: %s", d.Get(keyName), d.Id(), err)
	}

	return resourceAPIKeyRead(d, m)
}

func parseAPIKeyImportID(id string) (string, string, string, error) {
	parts := strings.SplitN(id, ":", 3)

	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("unexpected format of ID (%s), expected id:api_key_destination:on_behalf_of", id)
	}

	realID := parts[0]
	apiKeyDestination := parts[1]
	onBehalfOf := parts[2]

	return realID, apiKeyDestination, onBehalfOf, nil
}
