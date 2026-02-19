#!/bin/bash
# Script to verify the cvps up command implementation
# Run this when network connectivity is restored

set -e

echo "=== Verifying CVPS Up Command Implementation ==="
echo

echo "Step 1: Resolving dependencies..."
go mod tidy
echo "✓ Dependencies resolved"
echo

echo "Step 2: Running tests..."
go test -v ./internal/cmd -run TestRunUp
go test -v ./internal/cmd -run TestSaveLoadLocalContext  
go test -v ./internal/cmd -run TestGetCurrentSandboxID
echo "✓ Tests passed"
echo

echo "Step 3: Running linter..."
if command -v golangci-lint &> /dev/null; then
    golangci-lint run ./internal/cmd/up.go ./internal/cmd/up_test.go
    echo "✓ Lint passed"
else
    echo "⚠ golangci-lint not found, skipping"
fi
echo

echo "Step 4: Building..."
go build ./...
echo "✓ Build successful"
echo

echo "=== All verification steps completed! ==="
echo
echo "The cvps up command is ready to use:"
echo "  - cvps up"
echo "  - cvps up --name my-project"
echo "  - cvps up --cpu 4 --memory 8 --storage 50"
echo "  - cvps up --detach"
