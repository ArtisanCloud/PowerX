CONFIG_FILE := /app/etc/powerx.yaml

app-init:
	app-migrate
	app-seed
	app-run

app-migrate:
	powerxctl database migrate -f $(CONFIG_FILE)

app-seed:
	powerxctl database seed -f $(CONFIG_FILE)

app-run:
	./powerx -f $(CONFIG_FILE)

build-goctl-powerx-apis:
	goctl api go -api ./api/powerx.api -dir .
