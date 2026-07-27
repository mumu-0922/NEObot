# Component Guidelines

> React components render product state; reusable domain rules belong in
> `lib/`, I/O belongs in `services/`, and shared state belongs in `store/`.

## Component Structure

- Use function components. Feature components commonly use a named props
  interface followed by a default export; shared primitives may use named
  exports and `React.forwardRef`.
- Interactive modules declare `"use client"` at the top. Keep server-capable
  pages/layouts free of client-only imports when possible.
- Normalize cheap display input before rendering and return `null` for empty
  states. `components/chat/FollowUpQuestions.tsx` trims and filters questions
  before building the list.
- Extract deterministic logic when it deserves direct tests. For example,
  `components/ui/AnchoredPortal.tsx` exports
  `computeAnchoredPortalStyle`, tested by `__tests__/anchoredPortal.test.ts`.

```typescript
interface FollowUpQuestionsProps {
  questions: string[];
  onClick: (question: string) => void;
}

const FollowUpQuestions: React.FC<FollowUpQuestionsProps> = ({
  questions,
  onClick,
}) => {
  // render
};
```

## Props and Composition

- Name props types `<ComponentName>Props`. Keep them next to the component
  unless another module genuinely consumes the contract.
- Use precise unions for closed visual choices. `ButtonProps` in
  `components/ui/primitives.tsx` uses `variant` and `size` unions rather than
  arbitrary strings.
- Extend native React attributes for primitives and spread remaining props so
  callers retain standard DOM behavior:

  ```typescript
  export type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: "primary" | "secondary" | "danger" | "ghost";
  };
  ```

- Use `React.ReactNode` for composition slots. Event callbacks use `on*` names;
  state booleans use readable names such as `open`, `disabled`, or `isVisible`.
- Prefer existing primitives from `components/ui/` and existing product-area
  components before introducing another control with duplicate behavior.

## Styling

- Styling is Tailwind CSS 4 utility composition with theme tokens and global
  rules in `src/app/globals.css`; there are no CSS Modules or CSS-in-JS layer.
- Preserve light/dark variants, focus-visible rings, responsive variants, and
  `motion-reduce` behavior already present in nearby components.
- Shared primitives combine conditional class strings locally and accept a
  `className` override. Keep the override last so callers can extend the base.
- Reuse CSS variables such as `--background`, `--foreground`, and semantic
  Tailwind tokens instead of adding isolated hard-coded theme systems.

## Accessibility Contract

- Interactive icons need an accessible name. `IconButton` requires `label`
  and emits both `aria-label` and a title.
- Use native `button`, `input`, `label`, `section`, and list semantics before
  adding ARIA. Buttons explicitly use `type="button"` unless they submit a form.
- Associate labels and descriptions with stable IDs (`useId` is used by
  `Field`, `Tooltip`, and `FollowUpQuestions`).
- Dialogs and floating UI must preserve keyboard and focus behavior: Escape to
  close, focus trapping/restoration where modal, outside-pointer handling, and
  viewport clamping. See `components/ui/primitives.tsx` and
  `components/ui/AnchoredPortal.tsx`.
- Announce changing status when needed with `aria-live`, preserve the chat skip
  link/main-region pair, and keep mobile safe-area handling. These contracts
  are asserted in `__tests__/chatShellA11y.test.ts`.

## Common Mistakes

- Reading an entire Zustand store in a presentational leaf instead of using a
  narrow selector or feature hook.
- Starting network/persistence workflows directly in generic `ui/` primitives.
- Adding clickable `div` elements without button semantics or keyboard paths.
- Using browser globals from a Server Component or during SSR without a client
  boundary/fallback.
- Rendering untrusted generated HTML without the existing sanitization path in
  `components/content/MarkdownRenderer.tsx`.
