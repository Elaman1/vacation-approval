# Optional shortcuts; every target also has a Docker Compose equivalent.
.PHONY: up status verify test logs down

COMPOSE := docker compose

up:
	$(COMPOSE) up -d --build

status:
	$(COMPOSE) ps -a

verify:
	$(COMPOSE) config --quiet
	$(COMPOSE) run --rm smoke

# Tests run inside the pinned Go image; no local Go installation is required.
test:
	$(COMPOSE) run --rm tests

logs:
	$(COMPOSE) logs -f --tail=100 api worker outbox-publisher consumer deploy-processes

# Intentionally preserves PostgreSQL, Kafka, ClickHouse and Keycloak volumes.
down:
	$(COMPOSE) down
