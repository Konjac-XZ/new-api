# New API - Project Overview

## Project Purpose
New API is a next-generation large model gateway and AI asset management system. It's a fork/enhancement of the One API project that provides:
- API gateway for multiple AI models (OpenAI, Claude, Gemini, etc.)
- User and token management
- Model routing and load balancing
- Payment and billing system
- Real-time monitoring dashboard
- Support for multiple authentication methods (Discord, LinuxDO, Telegram, OIDC)

## Tech Stack

### Backend
- **Language**: Go 1.25.1
- **Framework**: Gin (web framework)
- **Database**: SQLite (default), MySQL, or PostgreSQL
- **Cache**: Redis (optional)
- **Key Dependencies**:
  - gin-gonic/gin - HTTP framework
  - glebarez/sqlite - SQLite driver
  - go-redis/redis - Redis client
  - golang-jwt/jwt - JWT authentication
  - gorilla/websocket - WebSocket support
  - aws-sdk-go-v2 - AWS services (Bedrock)

### Frontend
- **Framework**: React 18.2.0
- **Build Tool**: Vite 5.2.0
- **Package Manager**: Bun
- **UI Library**: Semi Design (@douyinfe/semi-ui)
- **Styling**: Tailwind CSS 3
- **Charting**: VChart (VisActor)
- **Internationalization**: i18next
- **Key Dependencies**:
  - react-router-dom - Routing
  - axios - HTTP client
  - react-markdown - Markdown rendering
  - mermaid - Diagram rendering
  - marked - Markdown parser

## Project Structure
```
new-api/
├── main.go                 # Entry point
├── go.mod / go.sum        # Go dependencies
├── makefile               # Build commands
├── .air.toml             # Hot reload config
├── web/                  # Frontend (React)
│   ├── src/
│   ├── public/
│   ├── package.json
│   └── vite.config.js
├── controller/           # HTTP handlers
├── router/              # Route definitions
├── service/             # Business logic
├── model/               # Data models
├── middleware/          # HTTP middleware
├── dto/                 # Data transfer objects
├── relay/               # API relay logic
├── monitor/             # Monitoring system
├── logger/              # Logging
├── common/              # Shared utilities
├── constant/            # Constants
├── types/               # Type definitions
├── setting/             # Configuration
├── channelcache/        # Channel caching
└── docs/                # Documentation
```

## Code Style & Conventions

### Go
- Standard Go conventions
- Package-based organization
- Error handling with explicit error returns
- Middleware pattern for HTTP handlers
- DTO pattern for API requests/responses

### Frontend (React/JavaScript)
- React functional components with hooks
- ESLint configuration with react-app preset
- Prettier for code formatting
  - Single quotes for strings
  - Single quotes for JSX attributes
- i18next for internationalization
- Component-based architecture

## Development Commands
- **Frontend build**: `cd web && bun install && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat VERSION) bun run build`
- **Backend dev**: `go run main.go` (with hot reload via air)
- **Frontend dev**: `cd web && bun run dev`
- **Linting (frontend)**: `bun run lint` (prettier check)
- **Linting fix (frontend)**: `bun run lint:fix` (prettier write)
- **ESLint**: `bun run eslint`
- **ESLint fix**: `bun run eslint:fix`

## Deployment
- Docker and Docker Compose support
- Environment variables for configuration
- SQLite (default), MySQL, or PostgreSQL support
- Redis for caching (optional)
- Multi-instance deployment with SESSION_SECRET and CRYPTO_SECRET

## Key Features
- Multiple AI model support (OpenAI, Claude, Gemini, etc.)
- Token-based authentication
- Model routing and load balancing
- Real-time monitoring
- Payment integration (Stripe, 易支付)
- Caching support
- WebSocket support for real-time features
