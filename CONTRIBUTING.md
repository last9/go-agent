# Contributing to Last9 Go Agent

Thank you for your interest in contributing to the Last9 Go Agent!

## Development Setup

1. Fork and clone the repository:
```bash
git clone https://github.com/your-username/go-agent.git
cd go-agent
```

2. Install dependencies:
```bash
go mod download
```

3. Run tests:
```bash
go test ./...
```

## Adding a New Framework Integration

To add support for a new framework (e.g., Echo, Chi, Fiber):

1. Create a new package under `instrumentation/`:
```bash
mkdir -p instrumentation/yourframework
```

2. Implement the integration following the Gin example pattern:
   - Provide a `New()` function for drop-in replacement
   - Provide a `Middleware()` function for existing routers
   - Auto-initialize the agent if not already started

3. Add an example under `examples/yourframework/`

4. Update the README.md with the new framework

## Code Style

- Follow standard Go conventions
- Run `go fmt` before committing
- Add comments for exported functions
- Keep functions focused and small

## Testing

- Add tests for new functionality
- Ensure all tests pass: `go test ./...`
- Test with real applications when possible

## Pull Request Process

1. Create a feature branch: `git checkout -b feature/your-feature`
2. Make your changes
3. Add tests
4. Update documentation
5. Commit with clear messages
6. Push and create a pull request

## Questions?

Feel free to open an issue for:
- Bug reports
- Feature requests
- Questions about the codebase
- Documentation improvements

Thank you for contributing! 🎉
