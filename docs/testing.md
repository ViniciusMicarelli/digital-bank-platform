# Trying it out

Everything runs locally with Docker; nothing is hosted. The card network,
the Central Bank (DICT/SPI), the identity bureau and the boleto clearing are
simulated, and the web and mobile apps show *Ambiente de testes* boxes to
play them (receive a Pix from another bank, swipe the card, issue a boleto).

## Start

```bash
make up       # build and start the stack (first time: a few minutes)
make seed     # demo customers with data in every feature (safe to rerun)
make totp     # current TOTP codes of the analysts
```

`make up-obs` adds Grafana. `make down` stops everything; `docker compose down -v` also wipes the data.

## Environments

| URL | What |
|---|---|
| http://localhost:8081 | Web app: customers and, for analysts, the back office |
| http://localhost:8000 | REST API ([reference](api/README.md)) |
| `make mobile` | Mobile app with Expo: scan the QR code with Expo Go on a phone on the same network. The Expo dev server also forwards the API (`/v1/*`) to the gateway, so if the app loads, the API answers; `w` opens it in the browser. On iPhone, allow Expo Go to use the local network (Settings → Privacy → Local Network) |
| http://localhost:16686 | Jaeger: a trace per request across services |
| http://localhost:9090 | Prometheus: metrics and alert rules |
| http://localhost:3000 | Grafana (`make up-obs`), anonymous viewer |

## Accounts

Password for every account: `correct horse battery staple`.

| Login | Who | What to try |
|---|---|---|
| `ana@example.com` | Customer with R$ 4.5k, Pix key, virtual card, credit line with an open invoice | Pix (key, QR code, refund, schedule), card (reveal with step-up, wallet, purchases, dispute), invoice payment, the boleto printed by `make seed`, notifications live; her "LOJA ONLINE BRASIL" purchase is under dispute |
| `bruno@example.com` | Customer whose account an analyst blocked | Everything is refused while blocked; an unblock request waits for a second analyst |
| `carla@example.com` | Politically exposed person | Her KYC is in manual review: no account until an analyst decides |
| `analista@digitalbank.dev` | Analyst (TOTP) | Back office: KYC queue (Carla), customer files, card disputes (Ana), block accounts, ask to unblock |
| `revisora@digitalbank.dev` | Second analyst (TOTP) | Approve the unblock `analista` asked for (four-eyes: the requester cannot), audit trail with chain verification |

Analysts sign in with a code: run `make totp`, or add the secret to an
authenticator app (development only):

| Analyst | TOTP secret |
|---|---|
| analista@digitalbank.dev | `JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP` |
| revisora@digitalbank.dev | `KRSXG5CTMVRXEZLUKRSXG5CTMVRXEZLU` |

A code works once, and five wrong ones lock the login for 15 minutes.

New customers: sign up at http://localhost:8081/criar-conta. Any valid CPF
is approved; a CPF starting with `111` is rejected (irregular) and one
starting with `222` goes to manual review (PEP).

## Card numbers

Cards are issued with BIN `999999` (wallet tokens `999998`), which no real
issuer uses; the number, expiry and CVV are shown after confirming the
password. Purchases are made with *Passar o cartão* in the card screen.
