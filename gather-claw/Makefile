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
# Medic detects clawpoint-go.new and restarts the agent process (~2s).
hot:
	@mkdir -p dev-builds
	cd clawpoint-go && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ../dev-builds/clawpoint-go.new .
	@echo "Binary dropped to dev-builds/clawpoint-go.new — medic will hot-swap it"

# Hot-swap the bridge binary
bridge:
	@mkdir -p dev-builds
	cd clawpoint-go && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ../dev-builds/clawpoint-bridge.new ./cmd/bridge
	@echo "Bridge binary dropped to dev-builds/clawpoint-bridge.new"
