.PHONY: build dev logs shell clean

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
