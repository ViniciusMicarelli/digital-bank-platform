# @bank/core

TypeScript shared by [`apps/web`](../../apps/web) and [`apps/mobile`](../../apps/mobile): the API client (token refresh, idempotency keys, error envelopes), money as decimal strings, the Server-Sent Events parser with a browser and a React Native transport, Portuguese error messages and the API types. No dependencies; both apps import the sources directly (`@bank/core` alias) and the web app's Vitest runs its tests.
