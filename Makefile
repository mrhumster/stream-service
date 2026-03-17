IMAGE_NAME := xomrkob/stream
NAMESPACE := go-app
DEPLOYMENT := stream-service
VERSION ?= $(shell git describe --tags --always || echo "latest")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

MINIO_ROOT_USER=$(shell kubectl -n $(NAMESPACE) get secret minio-credentials -o jsonpath='{.data.MINIO_ROOT_USER}' 2>/dev/null | base64 -d)
MINIO_ROOT_PASS=$(shell kubectl -n $(NAMESPACE) get secret minio-credentials -o jsonpath='{.data.MINIO_ROOT_PASSWORD}' 2>/dev/null | base64 -d)
MINIO_USER_KEY=$(shell kubectl -n $(NAMESPACE) get secret minio-credentials -o jsonpath='{.data.MINIO_ACCESS_KEY}' 2>/dev/null | base64 -d)
MINIO_USER_SECRET=$(shell kubectl -n $(NAMESPACE) get secret minio-credentials -o jsonpath='{.data.MINIO_SECRET_KEY}' 2>/dev/null | base64 -d)

MINIO_BUCKET=go-app-bucket

.PHONY: all build push deploy init-minio clean

all: build push deploy

build:
	@echo "Building docker image $(IMAGE_NAME):$(VERSION)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_NAME):$(VERSION) \
		-t $(IMAGE_NAME):latest .

push:
	@echo "Pushing image $(IMAGE_NAME):$(VERSION)..."
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest

deploy:
	@echo "Updating K8s deployment..."
	kubectl -n $(NAMESPACE) set image deployment/$(DEPLOYMENT) \
		stream=$(IMAGE_NAME):$(VERSION)
	@echo "Success!"

test:
	go test -v ./...

logs:
	kubectl -n $(NAMESPACE) logs -f -l app=stream

init-minio:
	@MINIO_POD=$$(kubectl get pods -n go-app -l app=minio -o jsonpath='{.items[0].metadata.name}'); \
	echo "Config MinIO in POD $$MINIO_POD..."; \
	kubectl exec -n $(NAMESPACE) $$MINIO_POD -- /bin/sh -c "\
		mc alias set local http://localhost:9000 $(MINIO_ROOT_USER) $(MINIO_ROOT_PASS) && \
		mc admin user add local $(MINIO_USER_KEY) $(MINIO_USER_SECRET) || true && \
		mc admin policy attach local readwrite --user=$(MINIO_USER_KEY) && \
		mc mb local/$(MINIO_BUCKET) || true"
	@echo "Done! Used key: $(MINIO_USER_KEY)"
