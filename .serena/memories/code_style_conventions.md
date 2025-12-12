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
