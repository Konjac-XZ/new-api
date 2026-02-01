# Code Style & Conventions

## Go Backend

### Naming
- Package names: lowercase, single word if possible
- Functions: PascalCase for exported, camelCase for unexported
- Variables: camelCase
- Constants: UPPER_SNAKE_CASE or PascalCase

### Structure
- Use interfaces for abstraction
- Middleware pattern for HTTP handlers
- DTO pattern for API requests/responses
- Error handling: explicit error returns, no panic in production code
- Logging: use the project's logger package

### Constants & Configuration
- Extract magic numbers to named constants
- Group related constants in dedicated files (e.g., `constants.go`)
- Use descriptive names: `MaxRecords`, `WriteWait`, `PongWait`
- Document units in comments (e.g., `10 * time.Second`)

### Concurrency Patterns
- **Manager Pattern**: Use for global state management
  - Thread-safe singleton with `sync.Once`
  - Atomic operations for flags (`atomic.Bool`)
  - RWMutex for complex state
- **Event-Driven**: Decouple components with event channels
  - Producer emits events to channel
  - Consumer subscribes and processes events
  - Non-blocking sends with `select/default`

### Code Quality
- No debug statements (`log.Printf`, `console.log`) in production code
- No commented-out code - delete or use version control
- Extract duplicate code to shared functions/variables

### Example Pattern
```go
// Exported function
func GetUser(id string) (*User, error) {
    // implementation
}

// Unexported helper
func validateUserID(id string) bool {
    // implementation
}
```

## Frontend (React/JavaScript)

### Naming
- Components: PascalCase (e.g., `UserProfile.jsx`)
- Functions/variables: camelCase
- Constants: UPPER_SNAKE_CASE
- CSS classes: kebab-case

### React Patterns
- Functional components with hooks
- Use `useState`, `useEffect`, `useContext` for state management
- Props destructuring
- Conditional rendering with ternary or logical operators
- Extract large components (>300 lines) into smaller, focused components
- Use `React.memo` for performance optimization on frequently re-rendered components

### Constants & Configuration
- Extract magic numbers to named constants (e.g., `constants.js`)
- Group related constants: timing, thresholds, limits
- Use descriptive names: `DURATION_UPDATE_INTERVAL_MS`, `BODY_DISPLAY_LIMIT_BYTES`

### Code Quality
- No console statements (`console.log`, `console.warn`) in production code
- No commented-out code - delete or use version control
- Proper error handling - avoid silent catches

### Code Formatting
- **Prettier** enforces formatting:
  - Single quotes for strings: `'string'`
  - Single quotes for JSX: `<Component prop='value' />`
  - 2-space indentation (default)
  - Line length: 80 characters (default)

### Linting
- **ESLint** with react-app preset
- Run before committing: `bun run eslint:fix`

### Internationalization
- Use i18next for all user-facing strings
- Extract strings: `bun run i18n:extract`
- Keys follow pattern: `namespace:key.subkey`

### Example Component
```jsx
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

export function UserProfile({ userId }) {
  const { t } = useTranslation();
  const [user, setUser] = useState(null);

  return (
    <div className='user-profile'>
      <h1>{t('profile.title')}</h1>
      {user && <p>{user.name}</p>}
    </div>
  );
}
```

## API Design

### Request/Response
- Use DTO pattern for type safety
- Consistent error responses
- HTTP status codes: 200 (OK), 400 (Bad Request), 401 (Unauthorized), 404 (Not Found), 500 (Server Error)

### Naming
- Endpoints: lowercase, kebab-case
- Query parameters: camelCase
- JSON fields: camelCase

## Database

### Naming
- Tables: snake_case, plural (e.g., `users`, `api_keys`)
- Columns: snake_case
- Foreign keys: `{table}_id`

## Comments & Documentation

### Go
- Exported functions/types should have doc comments
- Complex logic should be explained
- Format: `// FunctionName does something`

### JavaScript/React
- Complex logic should be explained
- Component props should be documented
- Use JSDoc for complex functions

## Version Control

### Commits
- Clear, descriptive messages
- Reference issues if applicable
- Conventional commits preferred:
  - `feat:` new feature
  - `fix:` bug fix
  - `refactor:` code refactoring
  - `docs:` documentation
  - `chore:` maintenance
