.PHONY: test test-integration test-race clean

# Standard test runner against local environment
test:
	go test -v -count=1 ./...

# Integration test target: runs real PostgreSQL tests (fails if PG is missing)
test-integration:
	@if command -v docker >/dev/null 2>&1; then \
		echo "==> Running PostgreSQL 16 Integration Tests in Docker Compose..."; \
		docker compose -f docker-compose.test.yml down -v --remove-orphans; \
		docker compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from test-runner; \
		docker compose -f docker-compose.test.yml down -v; \
	else \
		echo "==> Docker not found; running tests directly against PostgreSQL with REQUIRE_PG=1..."; \
		REQUIRE_PG=1 go test -v -count=1 ./...; \
	fi

# Docker race-detector runner with GCC
test-race:
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from test-runner

clean:
	docker compose -f docker-compose.test.yml down -v --remove-orphans 2>/dev/null || true
