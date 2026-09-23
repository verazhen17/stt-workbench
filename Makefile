REGISTRY ?= gcr.io/media17-streaming/stt
IMAGE_TAG ?= $(shell git rev-parse main)
PLATFORM ?= linux/amd64
NAMESPACE ?= stag
KUSTOMIZE_DIR ?= deploy/k8s/overlays/stag

BACKEND_IMAGE := $(REGISTRY)/workbench-backend:$(IMAGE_TAG)
FRONTEND_IMAGE := $(REGISTRY)/workbench-frontend:$(IMAGE_TAG)

.PHONY: image-build image-push image-release update-deployment deploy help

image-build:
	docker build --platform $(PLATFORM) -t $(BACKEND_IMAGE) ./backend
	docker build --platform $(PLATFORM) -t $(FRONTEND_IMAGE) ./frontend

image-push: image-build
	docker push $(BACKEND_IMAGE)
	docker push $(FRONTEND_IMAGE)

update-deployment:
	perl -0pi -e 's/(newTag:\s*).*/$${1}$(IMAGE_TAG)/g' $(KUSTOMIZE_DIR)/kustomization.yaml

deploy: update-deployment
	kubectl apply -k $(KUSTOMIZE_DIR) -n $(NAMESPACE)

image-release: image-push deploy

help:
	@echo "make image-build    Build backend and frontend images"
	@echo "make image-push     Build and push both images"
	@echo "make deploy         Update image tags and apply to namespace $(NAMESPACE)"
	@echo "make image-release  Build, push, and deploy to namespace $(NAMESPACE)"
	@echo "                    Override tag with IMAGE_TAG=<main commit SHA>"
