.PHONY: build test vet fmt lint cover cover-html cover-report clean

BINARY := ccells
COVERAGE_DIR := /tmp/claude/coverage
COVERAGE_OUT := $(COVERAGE_DIR)/coverage.out

build:
	go build -o $(BINARY) ./cmd/ccells

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint: vet fmt

# Run tests with coverage and print summary
cover:
	@mkdir -p $(COVERAGE_DIR)
	go test -coverprofile=$(COVERAGE_OUT) ./... 2>&1 | grep -E 'ok|FAIL|coverage'
	@echo ""
	@echo "=== Coverage Summary ==="
	@go tool cover -func=$(COVERAGE_OUT) | grep 'total:'
	@echo ""
	@echo "=== Low Coverage Functions (< 50%) ==="
	@go tool cover -func=$(COVERAGE_OUT) | grep -v '100.0%' | awk '{ if ($$NF+0 < 50 && $$NF != "0.0%") print }' | sort -t'%' -k1 -n | head -20
	@echo ""
	@echo "=== Zero Coverage ==="
	@go tool cover -func=$(COVERAGE_OUT) | grep '0.0%' | wc -l | xargs -I{} echo "{} functions at 0%"

# Generate HTML coverage report and open in browser
cover-html: cover
	go tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"
	@open $(COVERAGE_DIR)/coverage.html 2>/dev/null || echo "Open $(COVERAGE_DIR)/coverage.html in your browser"

# Per-package coverage breakdown
cover-report: cover
	@echo ""
	@echo "=== Per-Package Coverage ==="
	@go test -cover ./... 2>&1 | grep -E 'ok|FAIL' | sort -t'%' -k2 -n

clean:
	rm -f $(BINARY)
	rm -rf $(COVERAGE_DIR)
