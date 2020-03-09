package sendgrid

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/pkg/errors"
	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go"
	"golang.org/x/sync/errgroup"
)

const (
	keyUsername    = "username"
	keyEmail       = "email"
	keyPassword    = "password"
	keyDestination = "destination"
	keyLength      = "length"
	keyDisabled    = "disabled"
	keyIPs         = "ips"
	keyDomain      = "domain"

	defaultDomainID       = "0"
	defaultPasswordLength = 16
)

var (
	createSubuserRate = time.Tick(5 * time.Second)
	deleteSubuserRate = time.Tick(5 * time.Second)
)

type subuser struct {
	Username string
	Email    string
	Disabled bool
}

func resourceSubuser() *schema.Resource {
	return &schema.Resource{
		Create: resourceSubuserCreate,
		Read:   resourceSubuserRead,
		Update: resourceSubuserUpdate,
		Delete: resourceSubuserDelete,
		Importer: &schema.ResourceImporter{
			State: func(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				realID, password, err := parseSubuserImportID(d.Id())
				if err != nil {
					return nil, err
				}

				d.Set(keyPassword, password)
				d.SetId(realID)

				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			keyUsername: &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			keyEmail: &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			keyPassword: &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				MaxItems: 1,
				MinItems: 1,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						keyDestination: &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						keyLength: &schema.Schema{
							Type:     schema.TypeInt,
							Optional: true,
							Default:  defaultPasswordLength,
							ForceNew: true,
						},
					},
				},
			},
			keyIPs: &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				MinItems: 1,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			keyDisabled: &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			keyDomain: &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  defaultDomainID,
			},
		},
	}
}

func resourceSubuserCreate(d *schema.ResourceData, m interface{}) error {
	d.Partial(true)

	username := d.Get(keyUsername).(string)
	email := d.Get(keyEmail).(string)
	ips := d.Get(keyIPs).(*schema.Set).List()

	passConfigList := d.Get(keyPassword).([]interface{})
	if len(passConfigList) > 1 {
		return fmt.Errorf("password block may appear only once")
	}

	passLength := defaultPasswordLength
	passDest := ""
	if len(passConfigList) == 1 {
		passConfig := passConfigList[0].(map[string]interface{})
		passDest = passConfig[keyDestination].(string)
		passLength = passConfig[keyLength].(int)
	}

	passwordBytes, err := genPassword(passLength)
	if err != nil {
		return err
	}

	err = writeFile(passDest, passwordBytes)
	if err != nil {
		return errors.Wrap(err, "unable to save generated password")
	}

	password := string(passwordBytes)

	payload := map[string]interface{}{
		"username": username,
		"email":    email,
		"password": password,
		"ips":      ips,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	apiKey := m.(*Config).APIKey
	request := sendgrid.GetRequest(apiKey, "/v3/subusers", sendgridAddress)
	request.Method = http.MethodPost
	request.Body = data

	_, err = doRequest(request, withStatus(http.StatusCreated), withRateLimit(createSubuserRate))
	if err != nil {
		return errors.Wrap(err, "failed to create subuser")
	}

	createStateConf := &resource.StateChangeConf{
		Pending:                   []string{statusWaiting},
		Target:                    []string{statusDone},
		Timeout:                   d.Timeout(schema.TimeoutCreate),
		Delay:                     defaultBackoff,
		MinTimeout:                defaultBackoff,
		ContinuousTargetOccurence: 3,
		Refresh: func() (interface{}, string, error) {
			user, err := getSubuser(apiKey, username)
			if l, ok := err.(ratelimitError); ok {
				time.Sleep(l.timeout)
				return nil, statusWaiting, nil
			} else if err != nil {
				return nil, "", err
			} else if user == nil {
				return nil, statusWaiting, nil
			}

			return user, statusDone, nil
		},
	}

	_, err = createStateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("error waiting for subuser (%s) to be created: %s", d.Id(), err)
	}

	d.SetPartial(keyUsername)
	d.SetPartial(keyEmail)
	d.SetPartial(keyPassword)

	isDisabled := d.Get(keyDisabled).(bool)
	if isDisabled {
		err = setDisabled(apiKey, username, isDisabled)
		if err != nil {
			return errors.Wrap(err, "failed to disable subuser")
		}

		d.SetPartial(keyDisabled)
	}

	domain := d.Get(keyDomain).(string)
	if domain != defaultDomainID {
		err = setDomain(apiKey, username, domain)
		if err != nil {
			return errors.Wrap(err, "failed to set authenticated domain")
		}

		d.SetPartial(keyDomain)
	}

	d.Partial(false)

	d.SetId(username)

	return resourceSubuserRead(d, m)
}

func resourceSubuserRead(d *schema.ResourceData, m interface{}) error {
	apiKey := m.(*Config).APIKey
	user, err := getSubuser(apiKey, d.Id())
	if err != nil {
		return err
	} else if user == nil {
		d.SetId("")
		return nil
	}

	domainID, err := getDomain(apiKey, user.Username)
	if err != nil {
		return errors.Wrap(err, "unable to get domain authentication for subuser")
	}

	ips, err := getIPs(apiKey, user.Username)
	if err != nil {
		return errors.Wrap(err, "unable to get IPs for subuser")
	}

	d.Set(keyUsername, user.Username)
	d.Set(keyEmail, user.Email)
	d.Set(keyDisabled, user.Disabled)
	d.Set(keyDomain, domainID)
	d.Set(keyIPs, ips)

	return nil
}

func resourceSubuserUpdate(d *schema.ResourceData, m interface{}) error {
	d.Partial(true)

	apiKey := m.(*Config).APIKey
	username := d.Get(keyUsername).(string)

	if d.HasChange(keyDisabled) {
		disabled := d.Get(keyDisabled).(bool)
		err := setDisabled(apiKey, username, disabled)
		if err != nil {
			return errors.Wrap(err, "failed to set user.disabled")
		}

		d.SetPartial(keyDisabled)
	}

	if d.HasChange(keyIPs) {
		ips := d.Get(keyIPs).(*schema.Set).List()
		data, err := json.Marshal(ips)
		if err != nil {
			return err
		}

		request := sendgrid.GetRequest(apiKey, fmt.Sprintf("/v3/subusers/%s/ips", username), sendgridAddress)
		request.Method = http.MethodPut
		request.Body = data

		_, err = doRequest(request, withStatus(http.StatusOK))
		if err != nil {
			return err
		}

		d.SetPartial(keyIPs)
	}

	if d.HasChange(keyDomain) {
		domainID := d.Get(keyDomain).(string)
		err := setDomain(apiKey, username, domainID)
		if err != nil {
			return errors.Wrap(err, "failed to set user.domain")
		}

		d.SetPartial(keyDomain)
	}

	d.Partial(false)

	var eg errgroup.Group

	if d.HasChange(keyDisabled) {
		eg.Go(func() error { return waitForSubuser(d, m) })
	}

	if d.HasChange(keyDomain) {
		eg.Go(func() error { return waitForDomain(d, m) })
	}

	if d.HasChange(keyIPs) {
		eg.Go(func() error { return waitForIPs(d, m) })
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	return resourceSubuserRead(d, m)
}

func resourceSubuserDelete(d *schema.ResourceData, m interface{}) error {
	apiKey := m.(*Config).APIKey
	request := sendgrid.GetRequest(apiKey, "/v3/subusers/"+d.Id(), sendgridAddress)
	request.Method = http.MethodDelete

	res, err := doRequest(request, withStatus(http.StatusNoContent), withRateLimit(deleteSubuserRate), withRetry(5))
	if err == nil || res.StatusCode == http.StatusNotFound {
		return nil
	}

	return errors.Wrap(err, "failed to delete subuser")
}

func setDisabled(apiKey, username string, disabled bool) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, `{"disabled":%t}`, disabled)

	request := sendgrid.GetRequest(apiKey, "/v3/subusers/"+username, sendgridAddress)
	request.Method = http.MethodPatch
	request.Body = buf.Bytes()

	_, err := doRequest(request, withStatus(http.StatusNoContent))
	if err != nil {
		return err
	}

	return nil
}

func setDomain(apiKey, username string, domain string) error {
	var uri string
	var method rest.Method
	var body []byte
	var queryParams map[string]string

	if domain == defaultDomainID {
		uri = "/v3/whitelabel/domains/subuser"
		method = rest.Delete
		queryParams = map[string]string{"username": username}
	} else {
		uri = "/v3/whitelabel/domains/" + domain + "/subuser"
		method = rest.Post

		var buf bytes.Buffer
		fmt.Fprintf(&buf, `{"username":%s}`, username)
		body = buf.Bytes()
	}

	request := sendgrid.GetRequest(apiKey, uri, sendgridAddress)
	request.Method = method
	request.Body = body
	request.QueryParams = queryParams

	_, err := doRequest(request, withStatus(http.StatusCreated), withStatus(http.StatusNoContent))
	if err != nil {
		return err
	}

	return nil
}

func getDomain(apiKey, username string) (string, error) {
	request := sendgrid.GetRequest(apiKey, "/v3/whitelabel/domains/subuser", sendgridAddress)
	request.QueryParams = map[string]string{"username": username}
	request.Method = http.MethodGet

	res, err := doRequest(request, withStatus(http.StatusOK))
	if err != nil {
		return "", errors.Wrap(err, "failed to query domain")
	}

	data := struct {
		ID int64 `json:"id"`
	}{}

	err = json.Unmarshal([]byte(res.Body), &data)
	if err != nil {
		return "", errors.Wrap(err, "failed to unmarshal domain query response")
	}

	return strconv.FormatInt(data.ID, 10), nil
}

func getIPs(apiKey, username string) ([]interface{}, error) {
	request := sendgrid.GetRequest(apiKey, "/v3/ips", sendgridAddress)
	request.Method = http.MethodGet

	// TODO: pagination
	request.QueryParams = map[string]string{
		"subuser": username,
		"limit":   "500",
	}

	res, err := doRequest(request, withStatus(http.StatusOK))
	if err != nil {
		return nil, errors.Wrap(err, "failed to query IPs")
	}

	data := []struct {
		IP string `json:"ip"`
	}{}

	err = json.Unmarshal([]byte(res.Body), &data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal IP query response")
	}

	ips := make([]interface{}, 0, len(data))
	for _, ip := range data {
		ips = append(ips, ip.IP)
	}

	return ips, nil
}

func getSubuser(apiKey, name string) (*subuser, error) {
	request := sendgrid.GetRequest(apiKey, "/v3/subusers/"+name, sendgridAddress)
	request.Method = http.MethodGet

	res, err := doRequest(request, withStatus(http.StatusOK), withStatus(http.StatusNotFound))
	if res != nil && res.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to query subuser")
	}

	var user subuser

	err = json.Unmarshal([]byte(res.Body), &user)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal subuser query response")
	}

	return &user, nil
}

func randomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func genPassword(length int) ([]byte, error) {
	const numbers = "0123456789"
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const validChars = numbers + letters + "!@#$%^&*()-_=+"

	bytes, err := randomBytes(length + 2)
	if err != nil {
		return nil, err
	}

	for i := 0; i < length; i++ {
		bytes[i] = validChars[bytes[i]%byte(len(validChars))]
	}

	// Sendgrid requires at least one letter and one number
	i := int(bytes[length]) % length
	i2 := int(bytes[length+1]) % length
	if i == i2 {
		i2 = (i2 + 1) % length
	}

	bytes[i] = numbers[bytes[length]%byte(len(numbers))]
	bytes[i2] = letters[bytes[length+1]%byte(len(letters))]

	return bytes[:length], nil
}

func parseSubuserImportID(id string) (string, []map[string]interface{}, error) {
	parts := strings.SplitN(id, ":", 3)

	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", nil, fmt.Errorf("unexpected format of ID (%s), expected id:password_destination:password_length", id)
	}

	realID := parts[0]
	passDest := parts[1]
	passLen, err := strconv.ParseInt(parts[2], 10, 32)
	if err != nil {
		return "", nil, fmt.Errorf("invalid password length: %s", parts[2])
	}

	return realID, []map[string]interface{}{
		map[string]interface{}{
			keyDestination: passDest,
			keyLength:      passLen,
		},
	}, nil
}

func waitForSubuser(d *schema.ResourceData, m interface{}) error {
	config := m.(*Config)
	username := d.Get(keyUsername).(string)
	disabled := d.Get(keyDisabled).(bool)
	email := d.Get(keyEmail).(string)

	createStateConf := &resource.StateChangeConf{
		Pending:                   []string{statusWaiting},
		Target:                    []string{statusDone},
		Timeout:                   d.Timeout(schema.TimeoutUpdate),
		Delay:                     defaultBackoff,
		MinTimeout:                defaultBackoff,
		ContinuousTargetOccurence: 3,
		Refresh: func() (interface{}, string, error) {
			user, err := getSubuser(config.APIKey, username)
			if l, ok := err.(ratelimitError); ok {
				time.Sleep(l.timeout)
				return nil, statusWaiting, nil
			} else if err != nil {
				return nil, "", err
			} else if user == nil {
				return nil, statusWaiting, nil
			} else if user.Username != username {
				return nil, statusWaiting, nil
			} else if user.Email != email {
				return nil, statusWaiting, nil
			} else if user.Disabled != disabled {
				return nil, statusWaiting, nil
			}

			return user, statusDone, nil
		},
	}

	_, err := createStateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("error waiting for subuser (%s) to become consistent: %s", d.Id(), err)
	}

	return nil
}

func waitForDomain(d *schema.ResourceData, m interface{}) error {
	config := m.(*Config)
	username := d.Get(keyUsername).(string)
	domain := d.Get(keyDomain).(string)

	createStateConf := &resource.StateChangeConf{
		Pending:                   []string{statusWaiting},
		Target:                    []string{statusDone},
		Timeout:                   d.Timeout(schema.TimeoutUpdate),
		Delay:                     defaultBackoff,
		MinTimeout:                defaultBackoff,
		ContinuousTargetOccurence: 3,
		Refresh: func() (interface{}, string, error) {
			gotDomain, err := getDomain(config.APIKey, username)
			if l, ok := err.(ratelimitError); ok {
				time.Sleep(l.timeout)
				return "", statusWaiting, nil
			} else if err != nil {
				return "", "", err
			} else if gotDomain != domain {
				return "", statusWaiting, nil
			}

			return domain, statusDone, nil
		},
	}

	_, err := createStateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("error waiting for domain for subuser (%s) to become consistent: %s", d.Id(), err)
	}

	return nil
}

func waitForIPs(d *schema.ResourceData, m interface{}) error {
	config := m.(*Config)
	username := d.Get(keyUsername).(string)
	ips := d.Get(keyIPs).(*schema.Set).List()

	createStateConf := &resource.StateChangeConf{
		Pending:                   []string{statusWaiting},
		Target:                    []string{statusDone},
		Timeout:                   d.Timeout(schema.TimeoutUpdate),
		Delay:                     defaultBackoff,
		MinTimeout:                defaultBackoff,
		ContinuousTargetOccurence: 3,
		Refresh: func() (interface{}, string, error) {
			gotIPs, err := getIPs(config.APIKey, username)
			if l, ok := err.(ratelimitError); ok {
				time.Sleep(l.timeout)
				return "", statusWaiting, nil
			} else if err != nil {
				return "", "", err
			} else if !sliceContentsAreEqual(gotIPs, ips) {
				return nil, statusWaiting, nil
			}

			return ips, statusDone, nil
		},
	}

	_, err := createStateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("error waiting for IPs for subuser (%s) to become consistent: %s", d.Id(), err)
	}

	return nil
}
