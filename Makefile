.PHONY: generate
generate:
	tailwindcss -i ./templates/input.css -o ./static/output.css --minify

.PHONY: build
build-local: generate
	go build -o dist/localsite

.PHONY: run
run-local: build-local
	./dist/localsite