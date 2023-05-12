output = ./bin

run:
	go run main.go
clear:
	rm -rf ./logs/*.*
	rm -rf ./data/*
build-linux:
	CGO_ENABLED=0 && GOOS=linux && GOARCH=amd64 && go build -o $(output)/vermouth
build-win:
	go build -o $(output)/vermouth.exe
build:
	protoc -I proto --go_out=. --go-grpc_out=. ./proto/*.proto