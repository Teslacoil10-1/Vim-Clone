.PHONY: build start run clean

start:
	./main
build:
	go build ./cmd/main.go
clean:
	rm ./main
run:
	go run ./cmd/main.go
