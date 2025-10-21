.PHONY: build
build:
	go build -o rsc-boundary

.PHONY: install
install:
	go install

.PHONY: test
test:
	@./rsc-boundary -path testdata

.PHONY: clean
clean:
	rm -f rsc-boundary
