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


.PHONY: help build test dev-docker-build dev-deploy-k8s clean fmt golangci-lint frontend-check helm-lint lint

help: ## Show this help message
	@echo "Motus - Make targets:"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the motus binary
	@echo "Building motus..."
	@go build -ldflags "$(LDFLAGS)" -o bin/motus ./cmd/motus
	@echo "Binary built: bin/motus"

fmt:
	go fmt ./...

golangci-lint:
	docker run --rm -v "$(PWD)":"$(PWD)" -w "$(PWD)" golangci/golangci-lint:latest golangci-lint run --timeout=5m

frontend-check:
	cd web && npm run check

lint: fmt golangci-lint frontend-check goreleaser-check helm-lint ## Run all linters and checks
	@echo "Linting complete!"

goreleaser-check:
	goreleaser check

helm-lint:
	helm lint ./charts/motus -f ./charts/motus/values.yaml

test: lint ## Run all tests
	@echo "Running tests..."
	@go test ./... -v

dev-docker-build: ## Build the dev image
	@echo "Building development Docker image..."
	@docker build -t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) -f Dockerfile.dev \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) \
		--build-arg BRANCH=$(BRANCH) \
		.

dev-deploy-k8s: dev-docker-build ## Build dev image, push to IMAGE_REGISTRY, and deploy to K8s
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
	@echo "Deploying to Kubernetes with immutable digest..."; \
	helm template motus ./charts/motus \
		--namespace motion \
		-f ./charts/motus/values-dev.yaml \
		--set image.repository="$(IMAGE_REGISTRY)/$(IMAGE_NAME)" \
		--set image.tag="$(IMAGE_TAG)" \
		--set image.digest="$$IMAGE_DIGEST" \
	| kubectl apply -n motion -f - --wait
	@echo ""
	@echo "Development deployment complete!"
	@echo ""
	@echo "Deployment info:"
	@kubectl get pods -n motion -l app=motus
	@echo ""
	@echo "Access at: https://motus.example.com (set in values-dev.yaml)"
	@echo "Demo login: demo@motus.local / demo"
	@echo "Admin login: admin@motus.local / admin"

dev-reset-database:
	@echo "Resetting database (deleting all data)..."
	@kubectl exec -n motion statefulset/motus-postgres -- psql -U motus -d motus -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO motus; CREATE EXTENSION postgis;"
	@echo "Database reset complete"

dev-full-deploy: dev-reset-database dev-deploy-k8s ## Reset the dev database and redeploy
	@echo "Full development deployment complete!"

clean: ## Clean build artifacts
	@rm -rf bin/ dist/ motus
	@echo "Cleaned"

.PHONY: generate
generate: ## Regenerate ogen code from docs/openapi.yaml
	go generate ./internal/api/...

.PHONY: update-scalar
update-scalar: ## Download the latest self-hosted Scalar bundle
	curl -fsSL https://cdn.jsdelivr.net/npm/@scalar/api-reference/dist/browser/standalone.js \
	     -o docs/scalar.js

.DEFAULT_GOAL := help
