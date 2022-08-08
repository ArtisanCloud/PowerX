build-all: build-dev build-dev-linux build-migrate-dev build-migrate-linux build-seed-dev
build-all-dev: build-dev build-dev-linux

build-docker-dev:
	docker-compose -f docker-compose-dev.yaml build
up-dev:
	docker-compose -f docker-compose-dev.yaml up
down-dev:
	docker-compose -f docker-compose-dev.yaml down

build-dev:
	mkdir -p ./configs/
	cp ./{environment.yml,cache.yml,database.yml,log.yml} ./configs/
	go build -o marketX-dev main.go

build-dev-linux:
	mkdir -p ./configs/
	cp ./{environment.yml,cache.yml,database.yml,log.yml} ./configs/
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o marketX-dev-linux main.go

build-migrate-dev:
	go build -o powerX-migrate-dev database/migrations/migrate.go

build-seed-dev:
	go build -o powerX-seed-dev database/seeds/*

build-migrate-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o powerX-migrate-linux database/migrations/migrate.go

build-env-example:
	mkdir -p ./configs_example/
	cp ./{environment.yml,cache.yml,database.yml,log.yml} ./configs_example/

change-version:
	sed -i "s/{{version}}/${RELEASE_VERSION}/g" ./config/version.go



migrate-tables:
	go run database/migrations/main.go

migrate-tables-refresh:
	go run database/migrations/main.go refresh

migrate-tables-refresh-seeds: migrate-tables-refresh seed-tables

seed-tables:
	go run database/seeds/*


test-authService-createTokenForAccount:
	go test tests/main_test.go  tests/service_createTokenForAccount_test.go