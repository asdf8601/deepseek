run:
	go run main.go help

test:
	go run main.go -chat hello hello
	go run main.go -rm hello
	go run main.go -new hola
	go run main.go -chat test hola
	go run main.go -ls

build:
	@rm deepseek &> /dev/null || true
	go build -o deepseek main.go

install: build
	cp -f deepseek ~/.local/bin
