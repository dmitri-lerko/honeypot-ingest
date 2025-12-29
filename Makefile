.PHONY: build test test-unit test-e2e lint docker-build docker-push run clean tidy

# Variables
BINARY_NAME=honeypot-ingest
IMAGE_NAME=honeypot-ingest
GCP_PROJECT?=your-gcp-project
GCP_REGION?=europe-west2
IMAGE_TAG?=latest

# Build
build:
	go build -o bin/$(BINARY_NAME) ./cmd/server

# Run locally
run:
	GCS_BUCKET=test-bucket go run ./cmd/server

# Tests
test: test-unit

test-unit:
	go test -v -race -cover ./internal/...

test-e2e:
	go test -v -tags=e2e ./e2e/...

test-all: test-unit test-e2e

# Lint
lint:
	golangci-lint run ./...

# Docker
docker-build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

docker-push: docker-build
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(IMAGE_NAME)/$(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(IMAGE_NAME)/$(IMAGE_NAME):$(IMAGE_TAG)

# Terraform
tf-init:
	cd terraform && terraform init

tf-plan:
	cd terraform && terraform plan

tf-apply:
	cd terraform && terraform apply

tf-destroy:
	cd terraform && terraform destroy

# Dependencies
tidy:
	go mod tidy

# Clean
clean:
	rm -rf bin/
	go clean -testcache
