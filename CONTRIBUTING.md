# Contributing to Netmaker Kubernetes Operator

Thank you for your interest in contributing to the Netmaker Kubernetes Operator! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Code Style](#code-style)
- [Submitting Changes](#submitting-changes)
- [Release Process](#release-process)

## Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/). By participating, you are expected to uphold this code.

## Getting Started

### Prerequisites

Before you begin, ensure you have the following installed:

- **Go** v1.22.0 or higher
- **Docker** v17.03 or higher
- **kubectl** v1.11.3 or higher
- **Kubernetes cluster** v1.11.3 or higher (or use Kind for local development)
- **Make** (for running Makefile targets)
- **Git** (latest version recommended)

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/netmaker-k8s-ops.git
   cd netmaker-k8s-ops
   ```
3. Add the upstream repository:
   ```bash
   git remote add upstream https://github.com/gravitl/netmaker-k8s-ops.git
   ```

## Development Setup

### 1. Install Dependencies

The Makefile will automatically download required tools:

```bash
# Install all development dependencies
make controller-gen
make kustomize
make envtest
make golangci-lint
```

### 2. Verify Setup

```bash
# Build the operator
make build

# Run tests
make test

# Run linter
make lint
```

### 3. Development Workflow

```bash
# 1. Generate manifests and code
make manifests generate

# 2. Format code
make fmt

# 3. Build
make build

# 4. Run tests
make test

# 5. Lint code
make lint
```

### 4. Running Locally

You can run the operator locally for development:

```bash
# Build the binary
make build

# Run the operator (requires kubeconfig)
./bin/manager
```

Or use the `run` target:

```bash
make run
```

### 5. Building Docker Images

```bash
# Build Docker image
make docker-build IMG=your-registry/netmaker-k8s-ops:dev

# Build and push (requires Docker login)
make docker-build-push IMG=your-registry/netmaker-k8s-ops:dev
```

## Making Changes

### Branch Naming

Create a branch for your changes:

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
# or
git checkout -b docs/your-documentation-update
```

Branch naming conventions:
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring
- `test/` - Test improvements
- `chore/` - Maintenance tasks

### Code Organization

- **API Definitions**: `api/v1/` - Custom resource definitions
- **Controllers**: `internal/controller/` - Kubernetes controllers
- **Proxy**: `internal/proxy/` - API proxy implementation
- **Webhook**: `internal/webhook/` - Mutating webhook
- **Models**: `internal/models/` - Data models
- **Main**: `cmd/main.go` - Operator entry point
- **Tests**: `test/` - End-to-end tests
- **Config**: `config/` - Kubernetes manifests and kustomize configs

## Testing

### Unit Tests

Run unit tests:

```bash
make test
```

This will:
- Generate manifests and code
- Format and vet code
- Run all unit tests
- Generate coverage report

### E2E Tests

Run end-to-end tests:

```bash
make test-e2e
```

**Note**: E2E tests require a Kubernetes cluster (Kind is recommended for local testing).

### Manual Testing

1. Build and push your changes:
   ```bash
   make docker-build IMG=your-registry/netmaker-k8s-ops:test
   docker push your-registry/netmaker-k8s-ops:test
   ```

2. Deploy to a test cluster:
   ```bash
   cd config/manager
   kustomize edit set image controller=your-registry/netmaker-k8s-ops:test
   cd ../..
   make deploy
   ```

3. Test your changes:
   ```bash
   # Check operator logs
   kubectl logs -n netmaker-k8s-ops-system -l control-plane=controller-manager -c manager

   # Verify CRDs
   kubectl get crds | grep netmaker

   # Test custom resources
   kubectl apply -f config/samples/
   ```

### Test Coverage

Ensure adequate test coverage for new code. Run tests with coverage:

```bash
make test
# Coverage report will be in cover.out
go tool cover -html=cover.out
```

## Code Style

### Go Code Style

1. **Format**: Run `make fmt` before committing
2. **Vet**: Run `make vet` to check for issues
3. **Lint**: Run `make lint` to check code quality
4. **Fix Linting Issues**: Run `make lint-fix` to auto-fix issues

### Code Conventions

- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions focused and concise
- Handle errors explicitly (avoid ignoring errors)

### Kubernetes Code Style

- Follow [Kubebuilder conventions](https://book.kubebuilder.io/)
- Use structured logging with `logr.Logger`
- Implement proper error handling and reconciliation
- Add appropriate RBAC permissions

### Commit Messages

Write clear, descriptive commit messages:

```
type(scope): subject

body (optional)

footer (optional)
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

Example:
```
feat(proxy): add user IP mapping support

Adds dynamic user IP mapping functionality to the API proxy,
allowing different WireGuard peers to be mapped to different
Kubernetes users with custom group memberships.

Fixes #123
```

## Submitting Changes

### Pull Request Process

1. **Update your fork**:
   ```bash
   git fetch upstream
   git checkout main
   git merge upstream/main
   ```

2. **Create your changes** on a feature branch (see [Making Changes](#making-changes))

3. **Ensure tests pass**:
   ```bash
   make test
   make lint
   ```

4. **Commit your changes** with clear commit messages

5. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

6. **Create a Pull Request** on GitHub:
   - Provide a clear title and description
   - Reference any related issues
   - Include test results and screenshots if applicable
   - Update documentation if needed

### Pull Request Checklist

Before submitting a PR, ensure:

- [ ] Code follows the project's style guidelines
- [ ] Tests pass locally (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] Code is properly formatted (`make fmt`)
- [ ] Documentation is updated (if needed)
- [ ] Commit messages follow conventions
- [ ] Branch is up to date with upstream/main

### PR Review Process

1. Maintainers will review your PR
2. Address any review comments
3. Keep the PR focused and easy to review
4. Respond to feedback in a timely manner
5. Once approved, maintainers will merge your PR

## Release Process

Releases are managed through GitHub Actions workflows:

1. **Tag Creation**: Tags are created using semantic versioning (v1.0.0)
2. **Automated Build**: Docker images are automatically built and pushed
3. **Release Notes**: GitHub releases are automatically created

See [Release Workflow](.github/workflows/release.yml) for details.

### For Maintainers

To create a release:

1. Update version in relevant files (if needed)
2. Create and push a tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
3. Or use the GitHub Actions workflow to create a tag and release

## Documentation

### Updating Documentation

- README.md: Main project documentation
- docs/: User guides and detailed documentation
- examples/: Example configurations and use cases
- Inline comments: Code documentation

When adding features:
1. Update relevant documentation
2. Add examples if applicable
3. Update README if it's a major feature

## Getting Help

- **Issues**: Open an issue on GitHub for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions
- **Documentation**: Check the [docs/](docs/) directory

## Additional Resources

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Go Best Practices](https://golang.org/doc/effective_go)
- [Netmaker Documentation](https://docs.netmaker.org/)

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (Apache License 2.0).

---

Thank you for contributing to Netmaker Kubernetes Operator! 🎉

