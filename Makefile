.PHONY: build

default:
	@rm -rf build
	@mkdir -p build
	@go build -o build/host ./host
	@go build -o build/plugin ./plugin
	@echo Done!
	@ls build/

run: default
	@PLUGIN_CMD=build/plugin ./build/host