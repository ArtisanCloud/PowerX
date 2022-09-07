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


build-authorization:
	go build -o powerX-authorization cmd/authorization/main.go cmd/authorization/openAPI.go

build-migrate:
	go build -o powerX-migrate cmd/database/migrations/main.go

build-migrate-dev:
	go build -o powerX-migrate-dev cmd/database/migrations/migrate.go

build-seed-dev:
	go build -o powerX-seed-dev cmd/database/seeds/*

build-migrate-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o powerX-migrate-linux cmd/database/migrations/migrate.go

build-env-example:
	mkdir -p ./configs_example/
	cp ./{environment.yml,cache.yml,database.yml,log.yml} ./configs_example/

change-version:
	sed -i "s/{{version}}/${RELEASE_VERSION}/g" ./config/version.go

# ------------------------------------------------------------------------------------------------------------------------

migrate-tables:
	./powerX-migrate

migrate-tables-refresh:
	./powerX-migrate refresh

migrate-tables-refresh-seeds: migrate-tables-refresh seed-tables

seed-tables:
	go run cmd/database/seeds/*


# ------------------------------------------------------------------------------------------------------------------------


convert-routes-to-openapi:
	./powerX-authorization convertRouts2OpenAPI

convert-openapi-to-permissions:
	./powerX-authorization convertOpenAPI2Permissions

convert-routes-to-permissions:
	./powerX-authorization convertRoutes2Permissions

convert-permissions-to-openapi:
	./powerX-authorization convertPermissions2OpenAPI


import-rbac-data:
	./powerX-authorization importRBACData
dump-rbac-data:
	./powerX-authorization dumpRBACData

import-permission-modules:
	./powerX-authorization importPermissionModules
dump-permission-modules:
	./powerX-authorization dumpPermissionModules

import-policy-rules:
	./powerX-authorization importPolicyRules
dump-policy-rules:
	./powerX-authorization dumpPolicyRules


init-rbac-roles-permission:
	./powerX-authorization initRBACRolesAndPermissions
init-system-roles:
	./powerX-authorization initSystemRoles

init-policies-byRBACPermissions:
	./powerX-authorization initPoliciesByRBACPermissions


# ------------------------------------------------------------------------------------------------------------------------

test-authService-createTokenForAccount:
	go test tests/main_test.go  tests/service_createTokenForAccount_test.go