run:
	go run main.go

test:
	go run main.go -chat hello
	go run main.go -rm hello
	go run main.go -new hola
	go run main.go -chat test hola
	go run main.go -ls

build:
	go run -o deepseek main.go
