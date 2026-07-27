# Type Safety

> TypeScript is strict, but data from requests, storage, imports, streams, and
> third-party providers remains untrusted until validated or normalized.

## Compiler and Import Rules

- `tsconfig.json` enables `strict`, `isolatedModules`, `noEmit`, bundler module
  resolution, and the `@/* -> src/*` alias.
- Use `import type` / `export type` for type-only dependencies. The root
  `src/types.ts` is a type-only barrel over domain contracts.
- Prefer inferred local types, literal unions, `as const`, and `satisfies` for
  static configuration. `src/config/defaults.ts` uses
  `as const satisfies ChatConfig` to keep literals narrow while checking shape.

## Type Organization

- Domain types live beside domain logic, for example
  `src/lib/chat/types.ts`, `src/lib/plugin/types.ts`, and
  `src/lib/knowledge/types.ts`.
- Shared UI types stay with the component unless part of a public primitive,
  such as `ButtonProps` in `components/ui/primitives.tsx`.
- API DTOs, inputs, capabilities, and stream events live in
  `src/services/api/client/types.ts`.
- `src/types.ts` re-exports commonly shared domain types; do not move API-only
  DTOs there merely to shorten an import.

## Runtime Validation and Normalization

- API route JSON boundaries use Zod schemas from `src/lib/api/schemas.ts`.
  High-risk objects use `.strict()`, size/count limits from `src/config/limits`,
  and refinements for policies such as remote attachment URLs and encrypted
  secrets.
- Parse before using a middleware-supplied untyped body:

  ```typescript
  const parsed = ChatRequestSchema.parse(body);
  return handleChatStream({
    modelName: parsed.modelName,
    history: parsed.history,
  });
  ```

- Stored/imported/provider values generally enter as `unknown` and pass through
  type guards or normalizers. Examples include `normalizeSession()` in
  `src/lib/chat/entities.ts`, `isSearchMode()` in `searchMode.ts`, and response
  normalizers in `src/services/api/client/server/`.
- Generic HTTP methods (`requestJson<T>`) provide a compile-time DTO contract,
  but response normalizers are still required where remote or legacy payloads
  are known to drift.

## Error and Optional-Field Patterns

- Catch values as `unknown`, then narrow with `instanceof Error`, a type guard,
  or a central normalizer such as `normalizeUnknownError()`.
- Use discriminated unions for state machines and output blocks. Examples are
  `MessageOutputBlock` and `ChatGenerationEvent` in `lib/chat/types.ts`.
- Preserve optionality at transport/storage boundaries. Build optional fields
  conditionally when absence differs from an explicit empty value.
- Use `Record<string, unknown>` for JSON-like extension metadata; narrow fields
  before consuming them.

## Assertions and `any`

`eslint.config.mjs` currently disables `@typescript-eslint/no-explicit-any`,
and compatibility code/tests contain targeted `any` assertions. Therefore
`any` is not globally forbidden by tooling. New production code should still
prefer `unknown` plus narrowing. Use an assertion only at a proven boundary and
keep it adjacent to validation/normalization; do not propagate asserted values
through the application.

## Avoid

- Treating TypeScript annotations as runtime validation.
- Casting raw JSON directly to a domain type when a schema/normalizer exists.
- Adding duplicate versions of an existing domain or DTO interface.
- Using non-null assertions to hide an unhandled loading/empty state.
- Widening literal configuration to `string` when a union or `satisfies` keeps
  the contract exact.
