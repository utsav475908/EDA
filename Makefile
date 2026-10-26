.PHONY: build test test-pg vet proto run images kind-up kind-down web-dev

build:
	go build -o bin/lp-server ./cmd/lp-server

test:
	go test ./...

# Store conformance suite against a throwaway PostgreSQL in Docker.
test-pg:
	docker rm -f lp-test-pg >/dev/null 2>&1 || true
	docker run -d --name lp-test-pg -e POSTGRES_PASSWORD=lp -p 55432:5432 postgres:16
	until docker exec lp-test-pg pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
	LP_TEST_DATABASE_URL='postgres://postgres:lp@localhost:55432/postgres?sslmode=disable' \
		go test -count=1 -v ./internal/store/pgstore; status=$$?; docker rm -f lp-test-pg >/dev/null; exit $$status

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
