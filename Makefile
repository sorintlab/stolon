.PHONY: all

all:
	for cmd in stolonctl sentinel proxy keeper; do go install github.com/gravitational/stolon/cmd/"$$cmd"; done
