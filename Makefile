.PHONY: build
build:
	go build

.PHONY: install
install:
	go install

.PHONY: test
test:
	@./go-rsc-boundary -path testdata

.PHONY: clean
clean:
	rm -f go-rsc-boundary

.PHONY: fmt
fmt:
	nix fmt
