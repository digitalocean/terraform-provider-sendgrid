Terraform-Provider-Sendgrid
===========================

A Terraform provider for management of Sendgrid resources. Currently, the following resources are supported:

* [sendgrid_api_key](#resource-sendgrid_api_key)
* [sendgrid_subuser](#resource-sendgrid_subuser)

Installation
------------

### From Official Release

1. Download the [latest release](https://github.com/digitalocean/terraform-provider-sendgrid/releases/latest) for your platform.
1. Unzip to your [local plugins directory](https://www.terraform.io/docs/configuration/providers.html#third-party-plugins). For example, on linux:

    ```
    unzip terraform-provider-sendgrid-linux_amd64_v0.0.1.zip -d ~/.terraform.d/plugins
    ```

Configuration Example
-------------------------------

```
# <main.tf>
provider "sendgrid" {
  api_key = "SG.abc123"
}

resource "sendgrid_subuser" "user1" {
  username = "my-account-subuser1"
  email    = "subuser1@example.org"

  # A secure password is generated and written to
  # the destination file.
  password {
    destination = "./output/user1.pass"
  }

  # IP addresses assigned to this subuser
  ips = [
    "255.255.255.255"
  ]
}

resource "sendgrid_api_key" "user1" {
  name = "my-account-subuser1"

  # This API key will be provisioned under my-account-subuser1
  on_behalf_of = sendgrid_subuser.user1.id

  # The API key will be written to the destination file
  destination  = "./output/user1.key"

  # This key will grant granted permission for the following actions
  scopes = [
    "mail.batch.create",
    "mail.batch.delete",
    "mail.batch.read",
    "mail.batch.update",
    "mail.send"
  ]
}

```

Configuration Reference
------------------
\* Denotes a required field

### provider "sendgrid"
| Field    | Type   | Description                                                                                                          |
|----------|--------|----------------------------------------------------------------------------------------------------------------------|
| api_key* | string | The API key used to interact with Sendgrid. This can also be supplied via the SENDGRID_API_KEY environment variable. |

Example
```
provider "sendgrid" {
  api_key = "SG.abc123"
}
```

### resource "sendgrid_api_key"
| Field        | Type        | Description                                                                                                                                                                       |
|--------------|-------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| destination* | string      | A file that will be created to store the newly created API key. If the full path does not exist, it will be created. Care should be taken to keep the contents of this file safe. |
| name*        | string      | The name used to describe the created API key.                                                                                                                                    |
| on_behalf_of | string      | The subuser under which to create the API key. Default is empty.                                                                                                                 |
| scopes*      | set(string) | A set of permissions given to the created API key. See the [Sendgrid Documentation](https://sendgrid.com/docs/API_Reference/Web_API_v3/API_Keys/api_key_permissions_list.html) for more information.                                                                           |

**Note** the resource will be destroyed and recreated if any of the `on_behalf_of` or `destination` fields are updated.

Example
```
resource "sendgrid_api_key" "user1" {
  name = "my-api-key"
  on_behalf_of = "my-account-subuser1"
  destination  = "./output/user1.key"
  scopes = [
    "mail.send"
  ]
}
```

Importing an existing API key
```
# import subuser on_behalf_of's key
terraform import sendgrid_api_key.apikey1 api_key_id:destination:on_behalf_of

# on_behalf_of may be left empty
terraform import sendgrid_api_key.apikey1 api_key_id:destination:
```

### resource "sendgrid_subuser"
| Field                 | Type    | Description                                                                                                                                                                          |
|-----------------------|---------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| disabled              | boolean | Set to true if this subuser should temporarily lose the ability to perform actions. Default is false.                                                                                |
| domain                | string  | The authenticated domain ID from which this user is allowed to send email. Note that this is the domain ID and *not* the domain name itself. Default is "0" (built-in Sendgrid ID).                                                                    |
| email*                | string  | The email address of the subuser.                                                                                                                                                    |
| password*             |         |                                                                                                                                                                                      |
| password.destination* | string  | A file that will be created to store the newly generated password. If the full path does not exist, it will be created. Care should be taken to keep the contents of this file safe. |
| password.length       | int     | The length of the password to be generated. Default is 16 characters.                                                                                                                |
| username*             | string  | The username of the subuser.                                                                                                                                                         |

**Note** the resource will be destroyed and recreated if any of the `email`, `password`, or `username` fields are updated.

Example
```
resource "sendgrid_subuser" "user1" {
  username = "my-account-subuser1"
  email    = "subuser1@example.org"

  password {
    destination = "./output/user1.pass"
    length = 32
  }

  domain = "112233"

  disabled = true

  ips = [
    "255.255.255.254",
    "255.255.255.255"
  ]
}
```

Importing an existing subuser
```
terraform import sendgrid_subuser.user1 username:password_destination:password_length
```

Contributing
============

Getting Started
---------------

This provider requires a working installation of Go that is compatible with [Go Modules](https://blog.golang.org/using-go-modules).

1. Clone the repository
    ```
    git clone https://github.com/digitalocean/terraform-provider-sendgrid.git
    ```
2. Build the `terraform-provider-sendgrid` plugin, which produces a binary in the current working directory.
    ```
    make build

    # Alternatively, build and init terraform
    make init
    ```
3. Write a `main.tf` configuration file, which can then be planned and applied with `make plan` and `make apply`.
4. Destroy any test resources with `make destroy`.

Running Acceptance Tests
------------------------

Acceptance tests currently require at least Sendgrid Pro account, in order to test subuser management with assigned IP addresses. The `SENDGRID_API_KEY` environment variable must be set to your Sendgrid API key, and `SENDGRID_TEST_IPS` must be set to a JSON array of IP addresses to be assigned to test subusers. For example:

```
SENDGRID_API_KEY="SG.abc123" SENDGRID_TEST_IPS='["255.255.255.255"]' make testacc
```

Creating a Release
------------------------

1. Set environment variable `RELEASE_VERSION=vX.Y.Z` where `X.Y.Z` follows [semantic versioning](https://semver.org/) guidelines.
1. Create and push a new tag. E.g., `git tag -a $RELEASE_VERSION -m "Create release $RELEASE_VERSION" && git push origin $RELEASE_VERSION`
1. Run `make release` to build the plugin binaries under the `bin/` directory and package them into `.zip` files for each target platform.
1. Create a new release on GitHub, and write an appropriate description. Ensure the tag version is set to the same value as `RELEASE_VERSION`.
1. Attach each `.zip` file as an asset.
1. Publish the release.
