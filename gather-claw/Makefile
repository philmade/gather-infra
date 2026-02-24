.PHONY: build dev logs shell clean hot bridge

build:
	docker build -t gather-claw:latest .

dev:
	docker compose up --build

logs:
	docker compose logs -f

shell:
	docker exec -it claw-dev sh

clean:
	docker compose down -v

# Hot-swap: cross-compile for Linux and drop into dev-builds/.
# Medic detects clay.new and restarts the agent process (~2s).
hot:
	@mkdir -p dev-builds
	cd clay && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ../dev-builds/clay.new .
	@echo "Binary dropped to dev-builds/clay.new — medic will hot-swap it"

# Hot-swap the bridge binary
bridge:
	@mkdir -p dev-builds
	cd clay && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ../dev-builds/clay-bridge.new ./cmd/bridge
	@echo "Bridge binary dropped to dev-builds/clay-bridge.new"
