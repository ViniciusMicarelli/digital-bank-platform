# Web app

The customer app of the Digital Bank: onboarding with KYC, account, Pix (send, request with a QR Code, pay a QR Code, keys, refunds), debit and credit cards (step-up to reveal, controls, wallets), credit invoices, boletos and live notifications. Portuguese UI, built on the public REST API ([docs/api](../../docs/api/README.md)).

React 19 · TypeScript · Vite · Tailwind CSS v4 · GSAP · Lucide · TanStack Query · React Router · Vitest · Playwright.

## Design

| | |
|---|---|
| Color with meaning | Semantic tokens ([`src/index.css`](src/index.css)): **jade** is the bank and money coming in; outgoing money stays neutral (as in current Brazilian banking apps); **red** only for failures and declines; **amber** for pending and scheduled; **violet** for credit products. Each token has a light and a dark value. |
| Themes | Light and dark, following the system by default, with Sistema / Claro / Escuro in the sidebar and in Ajustes. `public/theme.js` applies the choice before the first paint (no flash; it is a file because the CSP forbids inline scripts). |
| Type and icons | Inter (self-hosted, `@fontsource-variable/inter`) with tabular numbers for money; Lucide icons. |
| Cards | [`BankCard`](src/components/BankCard.tsx): chip, contactless mark, a light sheen that follows the pointer and a 3D tilt, and a flip to the back with the number and CVV after step-up. Sizes are container-relative (`cqw`), so it stays proportional from the hero to the card page. |
| Motion (GSAP) | Page transitions and staggered entrances ([`src/lib/motion.ts`](src/lib/motion.ts)), count-up balances and limits, a sliding tab indicator, toasts that spring in, an animated receipt check, floating cards on sign-in. Everything is skipped under `prefers-reduced-motion`. |
| Loading | Skeleton rows instead of spinners. |

## Run

```bash
make up          # the backend (repo root)
make web         # http://localhost:5173, proxies /v1 to the gateway
```

Or the production image, served by nginx with the API behind it: `make up` also starts it on http://localhost:8081.

## How it works

| Concern | Choice |
|---|---|
| API calls | [`src/lib/api.ts`](src/lib/api.ts) on top of [`@bank/core`](../../packages/ts-core): one origin (dev proxy / nginx), no CORS. Access token in memory, refresh token in `sessionStorage`; a 401 refreshes once and retries (single flight). Every money-moving request carries an `Idempotency-Key`, kept across retries of the same form. |
| Money | Decimal strings end to end ([`money.ts`](../../packages/ts-core/src/money.ts)): parsed from what people type (`1.234,56`), formatted `R$ 1.234,56`, added with `BigInt`. No floats. |
| Live notifications | `EventSource` cannot send an `Authorization` header, so [`sse.ts`](../../packages/ts-core/src/sse.ts) reads Server-Sent Events over `fetch`, reconnecting with `Last-Event-ID`. Each notification shows a toast and refreshes balances and lists; the statement (a projection built by another service) is read again a moment later. |
| Sensitive actions | Revealing card data, adding or resuming a wallet token ask for the password in a modal ([`StepUp.tsx`](src/components/StepUp.tsx)); the 5-minute step-up token is reused for 4. |
| Routing | Signed out → login; signed in without a verified identity or an account → onboarding; then the app. |
| Errors | API codes become Portuguese messages ([`errors.ts`](../../packages/ts-core/src/errors.ts)). |
| Sandbox | Helpers that only exist in the test environment (receive a Pix from another bank, pass the card, close an invoice, issue a boleto, play the KYC analyst) sit in dashed "Ambiente de testes" boxes. |
| Hosting | nginx: SPA fallback, hashed assets cached for a year, CSP and other security headers, unbuffered proxy for the SSE stream. |

## Test

```bash
make web-test    # lint (oxlint), typecheck, unit tests (Vitest), build
make web-e2e     # browser tests (Playwright, desktop and mobile) against :8081
```

The browser tests sign up real customers through the screens, pass KYC, receive and send Pix, use the card with step-up, open credit, pay a boleto and check the live notifications. CI runs them against the web image after the backend e2e.
