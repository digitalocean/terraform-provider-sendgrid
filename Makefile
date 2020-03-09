# Go parameters
GOBUILD=go build
GOCLEAN=go clean
TEST?=$$(go list ./... |grep -v 'vendor')
GOTEST=go test
BINARY_NAME=terraform-provider-sendgrid

# Terraform parameters
TF=terraform
TF_LOG="TRACE"

all: test build

$(BINARY_NAME): main.go sendgrid/*.go
	$(GOBUILD) -o $(BINARY_NAME) -v

build: $(BINARY_NAME)

test:
	$(GOTEST) -i $(TEST) -timeout=30s

testacc:
	TF_ACC=1 $(GOTEST) $(TEST) -v -timeout 5m -count=1

clean-all:
	$(GOCLEAN)
	rm -rf $(BINARY_NAME) terraform.tfstate terraform.tfstate.backup test-output/* bin/

clean:
	rm -rf terraform.tfstate terraform.tfstate.backup test-output/*

init: build
	$(TF) init

plan:
	$(TF) plan

apply:
	TF_LOG=$(TF_LOG) $(TF) apply

destroy:
	$(TF) $(TF_ARGS) destroy

release:
	scripts/build-release.sh $(BINARY_NAME) $(RELEASE_VERSION)
	scripts/package-release.sh $(BINARY_NAME) $(RELEASE_VERSION)

.PHONY: all build test testacc clean clean-all init plan apply release