# Mobile app

The Digital Bank on iOS and Android: onboarding with KYC, home with balance and statement (pull to refresh), Pix (send with key lookup, keys, QR Code request and share, copia e cola), cards (password sheet to reveal the number, controls, simulated purchases), boletos, credit and invoices, notifications that arrive live as a banner, and preferences.

Expo SDK 58 · React Native · Expo Router · Reanimated · TanStack Query · `@bank/core` (shared with the web app, from [`packages/ts-core`](../../packages/ts-core)).

## Design

The same semantic tokens as the web app (jade for the bank and money in, neutral for money out, red only for failures, amber for pending, violet for credit), with light and dark values in [`src/lib/theme.tsx`](src/lib/theme.tsx). The theme follows the system, or Sistema / Claro / Escuro in Mais › Aparência (saved on the device). Inter via `@expo-google-fonts/inter`, Lucide icons.

GSAP needs a DOM, so the app animates with **Reanimated** (the React Native standard): cards enter in a cascade, buttons spring when pressed, the segmented control's indicator slides, the card flips to its back after step-up, notification banners slide in, balances count up, skeletons pulse while loading, and the sign-in cards float. Reanimated honors the system's reduced-motion setting.

## Run on a phone

```bash
make up        # the backend (repo root)
make mobile    # starts Expo; scan the QR code with Expo Go
```

The phone must be on the same Wi-Fi: the app calls the gateway at the address of the computer running Expo, port 8000 (or set `EXPO_PUBLIC_API_URL`). Everything it uses ships in Expo Go, so no native build is needed for development; store builds go through EAS (`npx eas-cli build`). `ios/` and `android/` are generated (`npx expo prebuild`) and not committed.

## Notes

| Concern | Choice |
|---|---|
| Session | Refresh token in the Keychain / Keystore (`expo-secure-store`, `WHEN_UNLOCKED_THIS_DEVICE_ONLY`), restored before the first screen; access token in memory. |
| Navigation | `Stack.Protected` keeps signed-out and signed-in screens apart; the tab layout sends customers without a verified identity or an account to onboarding. |
| Live notifications | React Native's `fetch` does not stream, so the SSE transport is an incremental `XMLHttpRequest` (`xhrTransport` in `@bank/core`); the web build uses fetch streaming. |
| Idempotency | Every money-moving request carries an `Idempotency-Key` (`crypto.randomUUID` from `expo-crypto`), kept while the same form is retried. |
| Sandbox | Test-only helpers (receive a Pix from another bank, pass the card, close an invoice, approve a KYC review) sit in dashed "Ambiente de testes" boxes. |

## Test

```bash
make mobile-test   # expo lint, typecheck, expo-doctor
make mobile-e2e    # web build + Playwright at a phone viewport against the stack
```
