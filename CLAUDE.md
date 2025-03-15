# Go NodeJS Library Guide

## Build Commands
- Build: `make` or `go build -v`
- Install dependencies: `make deps` or `go get -v -t .`
- Run tests: `make test` or `go test -v`
- Run single test: `go test -v -run TestName` (ex: `go test -v -run TestNewPool`)
- Format code: `goimports -w -l .`

## Code Style Guidelines
- **Formatting**: Use goimports for auto-formatting and import organization
- **Naming**: Go standard - CamelCase for exported, camelCase for unexported
- **Error Handling**: Always check errors explicitly, return them upstream
- **Imports**: Group standard library first, then third-party, then local
- **Types**: Use clear type names, prefer interfaces for flexibility
- **Context**: Support context-based execution where appropriate
- **Comments**: Document exported functions, types, and constants
- **Testing**: Write tests for all public functionality

## Structure
This codebase is a Go library for seamless NodeJS integration, allowing Go programs to run JavaScript through managed NodeJS instances with pooling support.