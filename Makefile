IMAGE_REGISTRY ?= reg.meh.wf
IMAGE_NAME ?= motus
IMAGE_TAG ?= dev

# Version info injected at build time
VERSION ?= dev
COMMIT  := $$(git rev-parse HEAD 2>/dev/null || echo unknown)
DATE    := $$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
BRANCH  := $$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)

LDFLAGS := -s -w \
	-X github.com/tamcore/motus/internal/version.Version=$(VERSION) \
	-X github.com/tamcore/motus/internal/version.Commit=$(COMMIT) \
	-X github.com/tamcore/motus/internal/version.BuildDate=$(DATE) \
	-X github.com/tamcore/motus/internal/version.Branch=$(BRANCH)


.PHONY: help build test dev-deploy-k8s clean fmt vet golangci-lint frontend-check helm-lint lint

help: ## Show this help message
	@echo "Motus - Make targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the motus binary
	@echo "Building motus..."
	@go build -ldflags "$(LDFLAGS)" -o bin/motus ./cmd/motus
	@echo "Binary built: bin/motus"

fmt:
	go fmt ./...

vet:
	go vet ./...

golangci-lint:
	docker run --rm -v "$(PWD)":"$(PWD)" -w "$(PWD)" golangci/golangci-lint:latest golangci-lint run --timeout=5m

frontend-check:
	cd web && npm run check

lint: fmt vet golangci-lint frontend-check goreleaser-check helm-lint ## Run all linters and checks
	@echo "Linting complete!"

goreleaser-check:
	goreleaser check

helm-lint:
	helm lint ./charts/motus -f ./charts/motus/values.yaml

test: lint ## Run all tests
	@echo "Running tests..."
	@go test ./... -v

dev-deploy-k8s: ## Build dev image, push to IMAGE_REGISTRY, and deploy to K8s
	@echo "Building development Docker image..."
	@docker build -t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) -f Dockerfile.dev \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) \
		--build-arg BRANCH=$(BRANCH) \
		.
	@echo ""
	@echo "Pushing to $(IMAGE_REGISTRY)..."
	@docker push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	@echo ""
	@IMAGE_DIGEST=$$(docker inspect $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) --format='{{index .RepoDigests 0}}' | cut -d'@' -f2); \
	echo "Using digest: $$IMAGE_DIGEST"
	@echo ""
	@echo "Deleting existing deployments to avoid stale-pod interference..."
	@kubectl delete deployment,job -n motion -l app.kubernetes.io/instance=motus --ignore-not-found --wait
	@echo ""
	echo "Deploying to Kubernetes with immutable digest..."; \
	helm upgrade --install motus ./charts/motus \
		--namespace motion \
		-f ./charts/motus/values-dev.yaml \
		--set image.repository="$(IMAGE_REGISTRY)/$(IMAGE_NAME)" \
		--set image.tag="$(IMAGE_TAG)" \
		--set image.digest="$$IMAGE_DIGEST" \
		--wait \
		--timeout=5m
	@echo ""
	@echo "Development deployment complete!"
	@echo ""
	@echo "Deployment info:"
	@kubectl get pods -n motion -l app=motus
	@echo ""
	@echo "Access at: https://motus.example.com (set in values-dev.yaml)"
	@echo "Demo login: demo / demo"
	@echo "Admin login: admin / admin"

DUMP ?= traccar_dump_20260215.sql
EMAIL ?= admin@motus.local

IMPORT_DAYS ?= 60
IMPORT_POSITIONS ?= 0
GEOCODE_LAST_N ?= 100
EXCLUDE_UNKNOWN ?= true

import-data: ## Import last 60 days from Traccar dump (override: DUMP=file.sql EMAIL=user@example.com FILTER=name)
	@echo "Importing Traccar data (last 60 days)..."
	@kubectl port-forward -n motion statefulset/motus-postgres 5437:5432 > /dev/null 2>&1 & \
	PF_PID=$$! ; \
	sleep 3 && \
	go run ./cmd/motus/ import \
		--source-dump=$(DUMP) \
		--target-host=localhost \
		--target-port=5437 \
		--target-db=motus \
		--target-user=motus \
		--target-password=motus123 \
		--admin-email=$(EMAIL) \
		$(if $(FILTER),--device-filter=$(FILTER),) \
		--recent-days=$(IMPORT_DAYS) \
		--max-positions=$(IMPORT_POSITIONS) \
		--geocode-last-n=$(GEOCODE_LAST_N) \
		--exclude-unknown=$(EXCLUDE_UNKNOWN) ; \
	kill $$PF_PID 2>/dev/null || true
	@echo "Import complete"

dev-reset-database:
	@echo "Resetting database (deleting all data)..."
	@kubectl exec -n motion statefulset/motus-postgres -- psql -U motus -d motus -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO motus; CREATE EXTENSION postgis;"
	@echo "Database reset complete"

dev-full-deploy: dev-reset-database dev-deploy-k8s ## Full dev deployment with data import
	@echo "Full development deployment complete!"

clean: ## Clean build artifacts
	@rm -rf bin/ dist/ motus
	@echo "Cleaned"

.DEFAULT_GOAL := help
