SRC_FILES := $(shell find . -name "*.go")

senatus: $(SRC_FILES)
	go build .
