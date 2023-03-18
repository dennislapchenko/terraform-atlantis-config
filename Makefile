VERSION=0.1.6
PATH_BUILD=build/
FILE_COMMAND=terraform-atlantis-config
FILE_ARCH=darwin_amd64

# Determine the arch/os combos we're building for
XC_ARCH=amd64 arm
XC_OS=linux darwin windows

.PHONY: clean
clean:
	rm -rf ./build
	rm -rf '$(HOME)/bin/$(FILE_COMMAND)'

.PHONY: gotestsum
gotestsum:
	mkdir -p cmd/test_artifacts
	gotestsum
	rm -rf cmd/test_artifacts

.PHONY: test
test:
	mkdir -p cmd/test_artifacts
	go test -v ./...
	rm -rf cmd/test_artifacts

.PHONY: version
version:
	@echo $(VERSION)

.PHONY: sign
sign:  build-all
	rm -f $(PATH_BUILD)${VERSION}/SHA256SUMS
	shasum -a256 $(PATH_BUILD)${VERSION}/* > $(PATH_BUILD)${VERSION}/SHA256SUMS

.PHONY: install
install:
	install -d -m 755 '$(HOME)/bin/'
	install $(PATH_BUILD)$(FILE_COMMAND)/$(VERSION)/$(FILE_COMMAND)_$(VERSION)_$(FILE_ARCH) '$(HOME)/bin/$(FILE_COMMAND)'
