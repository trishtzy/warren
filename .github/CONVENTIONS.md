# Conventions

## Commit messages

Use imperative mood, sentence case, under 72 characters. Reference the issue number in the body or title when applicable.

Format:

```
<Type> <short description> (#<issue>)
```

Types:
- **Add** — new feature or capability
- **Fix** — bug fix
- **Update** — enhancement to existing feature
- **Remove** — delete code or feature
- **Refactor** — restructure without behavior change
- **Docs** — documentation only
- **Chore** — maintenance, CI, deps, tooling

Examples:
```
Add post submission and listing (#5)
Fix duplicate URL detection on submit (#42)
Update CI to use PostgreSQL 18 (#14)
Chore: add PR template and conventions
```

## PR titles

Follow the same format as commit messages. Keep under 70 characters. The PR title becomes the squash-merge commit message, so make it count.

## Branch names

Format: `<type>/<short-description>`

Types: `feature/`, `fix/`, `chore/`, `refactor/`, `docs/`

Examples:
```
feature/post-submission
fix/ssrf-title-fetch
chore/pr-template-conventions
```
