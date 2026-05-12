.PHONY: clean build install help publish

# Prefer Corepack because it honors packageManager and keeps the SDK lockfile on
# the pnpm version expected by this package instead of a newer global pnpm.
PNPM ?= $(shell if command -v corepack >/dev/null 2>&1; then echo "corepack pnpm"; elif command -v pnpm >/dev/null 2>&1; then echo pnpm; else echo pnpm; fi)

help:
	@echo "Available commands:"
	@echo "  make clean      - Remove build artifacts and node_modules"
	@echo "  make build      - Build Node.js plugin"
	@echo "  make install    - Install dependencies"
	@echo "  make publish    - Publish to npm registry"

clean:
	$(PNPM) run clean

install: clean
	# Make recipes are non-interactive, so tell pnpm it may replace an incompatible node_modules layout without prompting.
	CI=true $(PNPM) install

build: install
	$(PNPM) run build

publish: build
	$(PNPM) run pub
	rm -rf dist/
