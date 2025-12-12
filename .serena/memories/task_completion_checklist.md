# Task Completion Checklist

When completing a task, follow this checklist:

## Code Quality
- [ ] Code follows project conventions (Go style, React patterns)
- [ ] No console errors or warnings
- [ ] Code is properly formatted
  - Frontend: Run `bun run lint:fix` and `bun run eslint:fix`
  - Backend: Go fmt is automatic

## Testing
- [ ] Frontend changes tested in browser
- [ ] Backend changes compile without errors
- [ ] No breaking changes to existing functionality

## Frontend Specific
- [ ] Run `bun run lint` - Prettier formatting check passes
- [ ] Run `bun run eslint` - ESLint check passes
- [ ] i18n strings updated if UI text changed: `bun run i18n:extract`
- [ ] Component follows React best practices (hooks, functional components)

## Backend Specific
- [ ] Code compiles: `go build -o ./tmp/main .`
- [ ] No unused imports
- [ ] Error handling is explicit
- [ ] Database migrations (if applicable) are included

## Git & Commits
- [ ] Changes are staged: `git add .`
- [ ] Commit message is clear and descriptive
- [ ] Commit follows conventional commits if applicable
- [ ] No sensitive data in commits (.env files, credentials)

## Documentation
- [ ] README updated if needed
- [ ] Comments added for complex logic
- [ ] API changes documented

## Final Verification
- [ ] All changes are intentional and complete
- [ ] No temporary debug code left
- [ ] No console.log or print statements left (unless intentional)
