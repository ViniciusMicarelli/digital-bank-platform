# Contributing to Digital Bank Platform

Thank you for your interest in contributing to this project!

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/your-username/digital-bank-platform.git
   ```
3. Create a new branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```
4. Install dependencies:
   ```bash
   make install
   ```
5. Start infrastructure:
   ```bash
   make infra-up
   ```
6. Run migrations:
   ```bash
   make migrate
   ```

## Development

### Running the Development Environment

```bash
make dev
```

This will start all services in development mode.

### Making Changes

1. Make your changes in a feature branch
2. Run tests before committing:
   ```bash
   make test
   ```
3. Lint your code:
   ```bash
   make lint
   ```
4. Format your code:
   ```bash
   make format
   ```

### Code Style Guidelines

- **TypeScript**: Use Prettier with the configured ESLint rules
- **Python**: Follow PEP 8 style guide
- **JavaScript**: Use ESLint with Prettier
- **Protobuf**: Follow Google's Style Guide

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `style:` - Code style changes (formatting)
- `refactor:` - Code refactoring
- `perf:` - Performance improvements
- `test:` - Adding or updating tests
- `chore:` - Maintenance tasks

Example:
```
feat(accounts): Add new account creation endpoint
```

## Pull Requests

1. Update your branch:
   ```bash
   git rebase main
   ```
2. Ensure all tests pass:
   ```bash
   make test
   ```
3. Ensure linting passes:
   ```bash
   make lint
   ```
4. Create a pull request
5. Request reviews from maintainers

## Architecture

This project follows a microservices architecture:

### Services
- **Ledger**: Core accounting and transaction management
- **Payment Gateway**: Payment processing and reconciliation
- **Identity**: User authentication and authorization
- **Accounts**: Account management and operations
- **API Gateway**: Entry point for all API requests

### Contracts
- **Protobuf**: Protocol buffer definitions
- **OpenAPI**: API specifications

### Infrastructure
- **PostgreSQL**: Primary database
- **Redis**: Caching and session management
- **Docker Compose**: Infrastructure orchestration

## Testing

### Unit Tests
```bash
make test:unit
```

### Integration Tests
```bash
make test:integration
```

### Contract Tests
```bash
make test:contracts
```

## Documentation

- Update API documentation for new endpoints
- Add examples to usage documentation
- Update architecture diagrams if needed

## Code of Conduct

- Be respectful and inclusive
- Focus on collaboration, not conflict
- Provide constructive feedback
- Be open to differing opinions

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## Questions?

Feel free to open an issue for any questions or concerns.
