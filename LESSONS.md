# Lessons Learned

Go template rendering: when multiple page templates define the same template name (e.g., "content"), parsing all files into one shared set causes conflicts. Solution: parse layout + each page file separately so each page gets its own template set.

Chi route ordering: specific routes must be registered before catch-all routes. E.g., POST /portfolios/{id}/delete must come before POST /portfolios, otherwise chi matches the catch-all first and the specific route never fires.

Pre-check tool availability: before writing implementation plans, verify that required tools (sqlc, mockery, goose) are available. If not, document the alternative approach in the plan to avoid drift during implementation.
