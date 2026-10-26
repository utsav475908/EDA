.PHONY: build test vet proto run images kind-up kind-down web-dev

build:
	go build -o bin/lp-server ./cmd/lp-server

test:
	go test ./...

vet:
	go vet ./...

# Generates Go stubs, grpc-gateway handlers and OpenAPI into gen/ (needs buf).
proto:
	buf lint && buf generate

# Local control plane with simulated compute; no Kubernetes required.
run:
	LP_RECONCILE_EVERY=2s go run ./cmd/lp-server

images:
	docker build -t lp/lp-server:dev .
	docker build -t lp/spark-runtime:3.5.3-lp1 runtimes/spark
	docker build -t lp/notebook:dev runtimes/notebook
	docker build -t lp/mlflow:2.18.0-lp1 runtimes/mlflow

kind-up:
	./deploy/kind/up.sh

kind-down:
	kind delete cluster --name lp

web-dev:
	cd web && npm install && npm run dev
