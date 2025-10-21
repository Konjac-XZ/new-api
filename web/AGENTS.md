# Repository Guidelines

## Project Structure & Module Organization
- `src/` hosts the React app: `components/` (UI building blocks), `pages/` (route screens), `services/` (API clients), `context/` and `hooks/` (shared state), `helpers/` and `constants/` (utilities), plus `i18n/` (locale bundles).
- `public/` serves static assets unchanged by Vite; `index.html` bootstraps the shell.
- `dist/` is generated output from `bun run build` and should not be touched manually.
- Root configs (`vite.config.js`, `tailwind.config.js`, `.eslintrc.cjs`, `.prettierrc.mjs`) apply globally—reuse them when extending tooling.

## Build, Test, and Development Commands
- `bun install` syncs dependencies with `bun.lock`.
- `bun run dev` starts the Vite dev server with HMR.
- `bun run build` emits the production bundle in `dist/`.
- `bun run preview` serves the built bundle for smoke tests.
- `bun run lint` runs Prettier; `bun run eslint` enforces ESLint (including license headers). Use the `:fix` variants before submitting.
- `bun run i18n:extract` regenerates locale catalogs; follow with `bun run i18n:sync` when sharing translations.

## Coding Style & Naming Conventions
- Prettier (via `@so1ve/prettier-config`) formats files—keep single quotes, 2-space indentation, and trailing commas it applies.
- Components, contexts, and pages use PascalCase filenames; hooks begin with `use`; helpers stay camelCase.
- Every `.js`/`.jsx` file must retain the GPL header enforced by `eslint-plugin-header`.
- Favor existing Semi UI primitives and Tailwind utilities; only place global styles in `src/index.css`.

## Testing Guidelines
- No automated suite exists today—add coverage alongside new work using Vitest + React Testing Library. Name files `Component.test.jsx` and colocate near the unit or under `src/__tests__/`.
- Stub API traffic via helpers in `src/services/` to keep tests deterministic, and document any gaps in the PR when automation is impractical.
- Always verify new flows manually with `bun run dev` before requesting review.

## Commit & Pull Request Guidelines
- Follow the Conventional Commits style in `git log` (`feat:`, `chore:`, `refactor:`); scopes remain optional but helpful.
- Keep PRs focused, link related issues, and explain impact plus rollout notes. Attach before/after screenshots for visible UI.
- Ensure `bun run lint`, `bun run eslint`, and any added tests pass locally. Re-run `bun run i18n:extract` whenever you add or rename locale strings.
