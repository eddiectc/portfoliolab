---
task: 04
feature: Project Skeleton + Portfolio CRUD
depends-on: [03]
status: pending
---
# Task 04: Portfolio Web UI (Server-Rendered Pages)

## Goal
Server-rendered HTML pages for portfolio management: list all portfolios, create new, edit existing. Uses Go templates with a shared base layout.

## Files to Create/Modify
| File | Action | Description |
|------|--------|-------------|
| `internal/web/renderer.go` | create | Template renderer with layout/partials support |
| `templates/base.html` | create | Base layout: HTML5, nav, content block, footer |
| `templates/partials/nav.html` | create | Navigation partial |
| `templates/portfolio/list.html` | create | List all portfolios with create/edit/delete links |
| `templates/portfolio/form.html` | create | Create/edit portfolio form |
| `templates/portfolio/detail.html` | create | Portfolio detail view |
| `internal/web/static/css/style.css` | create | Minimal CSS for layout, forms, tables |
| `internal/api/handlers/portfolio_web.go` | create | Web page handlers (ListPage, CreatePage, EditPage, DetailPage, DeletePage) |
| `internal/api/router.go` | modify | Add web routes alongside API routes |
| `internal/api/handlers/portfolio_web_test.go` | create | Template rendering tests |

## Implementation Steps
1. Create `internal/web/renderer.go`:
   - Parse templates once at startup (base + partials + page templates)
   - Execute with data context
   - Serve static assets from `internal/web/static/`
2. Create `templates/base.html` with:
   - HTML5 doctype, responsive meta tag
   - `<title>` block, `<style>` block
   - Navigation bar with links: Dashboard, Portfolios, Accounts, Transactions, Positions, Analytics, Import
   - Content block (`{{template "content" .}}`)
   - Footer
3. Create portfolio list page: table showing name, currency, created date, actions (edit/delete)
4. Create portfolio form page: name input, currency dropdown (common currencies), submit button
5. Create portfolio detail page: shows portfolio info, links to add accounts/transactions
6. Implement web handlers:
   - `GET /portfolios` → list page
   - `GET /portfolios/new` → create form
   - `POST /portfolios` → create (uses same API handler logic)
   - `GET /portfolios/{id}` → detail page
   - `GET /portfolios/{id}/edit` → edit form
   - `PATCH /portfolios/{id}` → update
   - `POST /portfolios/{id}/delete` → delete (POST for safety)
7. Add static file serving for CSS
8. Write template rendering tests (edge cases: empty list, special characters in names)

## Acceptance Criteria
- [ ] `GET /portfolios` renders a table of portfolios (empty state handled gracefully)
- [ ] `GET /portfolios/new` renders a form with name and currency fields
- [ ] Submitting the form creates a portfolio and redirects to list
- [ ] `GET /portfolios/{id}` shows portfolio details
- [ ] `GET /portfolios/{id}/edit` pre-fills the form with current values
- [ ] Delete shows confirmation or uses POST method
- [ ] Base template has working navigation (links can be placeholders for now)
- [ ] CSS provides clean, readable layout
- [ ] Templates handle edge cases: empty portfolio list, long names

## Testing Requirements
- Unit tests: Template rendering doesn't panic on empty data, special characters escaped
- Manual verification: Open `http://localhost:8080/portfolios` in browser, test full CRUD flow

## Notes
- Keep templates simple — no complex logic, no range over complex structs
- Use partials for nav bar (reusable across all pages)
- Form validation errors should display inline above the field
- Use `html/template` — never `text/template` for HTML (XSS protection)
- Static CSS should be minimal; no external CSS framework dependency
- Web handlers can share validation logic with API handlers by importing the domain service
