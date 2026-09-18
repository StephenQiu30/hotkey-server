# Generated API client

The files in this directory are produced directly from
`../docs/openapi/openapi.json` by running `pnpm openapi:generate`. Do not
hand-write endpoint functions or response types here. Umi OpenAPI writes the
generated client directly to `src/api/`; its functions use `src/request.ts` and
keep browser traffic on the same-origin `/api/*` path.
