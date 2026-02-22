.PHONY: build

BINARY := tmux-visualiser
CMD := ./src

build:
	go build -o $(BINARY) $(CMD)
