#!/usr/bin/env bash
# Walks through the public API of a running stack (`make up`):
# two customers open accounts, one is funded by a sandbox Pix, then sends a
# transfer and a Pix to another bank, and reads balance and statement.
set -euo pipefail

API=${API:-http://127.0.0.1:8000}
command -v jq >/dev/null || { echo "jq is required"; exit 1; }

step() { printf '\n\033[1m▶ %s\033[0m\n' "$*"; }
call() { # call METHOD PATH TOKEN [JSON] [IDEMPOTENCY_KEY]
  local args=(-sS -X "$1" "$API$2" -H "Authorization: Bearer $3" -H 'Content-Type: application/json')
  [[ -n "${4:-}" ]] && args+=(-d "$4")
  [[ -n "${5:-}" ]] && args+=(-H "Idempotency-Key: $5")
  curl "${args[@]}"
}
PASSWORD='correct horse battery staple'
# token NAME: signs up NAME@example.com and logs in; prints the access token.
token() {
  curl -sS -X POST "$API/v1/auth/register" -d "{\"email\":\"$1@example.com\",\"password\":\"$PASSWORD\",\"full_name\":\"$1\"}" >/dev/null
  curl -sS -X POST "$API/v1/auth/login" -d "{\"email\":\"$1@example.com\",\"password\":\"$PASSWORD\"}" | jq -r .access_token
}
uuid() { python3 -c 'import uuid; print(uuid.uuid4())'; }

SUFFIX=$(uuid | cut -c1-8)
step "Ana and Bruno sign up and log in (identity-service issues Ed25519 tokens)"
ANA=$(token "ana-$SUFFIX")
BRUNO=$(token "bruno-$SUFFIX")
echo "access token: ${ANA:0:40}…"

step "Onboarding: no account before identity verification (KYC)"
call POST /v1/accounts "$ANA" '{}' "$(uuid)" | jq -c .
# cpf: a random valid CPF (the bureau simulator approves it).
cpf() { python3 -c '
import random
d=[random.randint(3,9)]+[random.randint(0,9) for _ in range(8)]
for n in (9,10): d.append(sum(d[i]*(n+1-i) for i in range(n))*10%11%10)
print("".join(map(str,d)))'; }
profile() { # profile TOKEN NAME
  call PUT /v1/me/profile "$1" "{\"full_name\":\"$2\",\"cpf\":\"$(cpf)\",\"birth_date\":\"1990-05-20\",\"phone\":\"+5511912345678\",\"address\":{\"postal_code\":\"01310-100\",\"street\":\"Av. Paulista\",\"number\":\"1000\",\"city\":\"São Paulo\",\"state\":\"SP\"}}" >/dev/null
  call POST /v1/me/kyc "$1" | jq -c '{full_name, cpf, kyc_status}'
}
profile "$ANA" "Ana Souza"
profile "$BRUNO" "Bruno Lima"

step "They open accounts (with the verified name and CPF)"
ANA_ACC=$(call POST /v1/accounts "$ANA" '{}' "$(uuid)" | tee /dev/stderr | jq -r .id)
BRUNO_ACC=$(call POST /v1/accounts "$BRUNO" '{}' "$(uuid)" | jq -r .id)

step "Ana registers a Pix key and receives R\$ 500,00 from another bank (sandbox)"
KEY="ana.$SUFFIX@example.com"
call POST "/v1/accounts/$ANA_ACC/pix/keys" "$ANA" "{\"type\":\"EMAIL\",\"value\":\"$KEY\"}" | jq -c .
PIXIN=$(call POST /v1/sandbox/pix-in "$ANA" "{\"key\":\"$KEY\",\"amount\":\"500.00\",\"payer_name\":\"Empresa X\"}")
echo "$PIXIN" | jq -c '{status, amount, end_to_end_id}'
PIXIN_ID=$(echo "$PIXIN" | jq -r .id)

step "Ana transfers R\$ 120,00 to Bruno (sent twice with the same Idempotency-Key)"
IDEM=$(uuid)
for _ in 1 2; do
  call POST "/v1/accounts/$ANA_ACC/transfers" "$ANA" "{\"to_account_id\":\"$BRUNO_ACC\",\"amount\":\"120.00\",\"description\":\"Aluguel\"}" "$IDEM" | jq -c '{id, status, amount}'
done

step "Ana looks up a key at another bank and sends a Pix"
call GET /v1/pix/keys/maria@outrobanco.com.br "$ANA" | jq -c .
call POST "/v1/accounts/$ANA_ACC/pix/transfers" "$ANA" '{"key":"maria@outrobanco.com.br","amount":"80.00","description":"Presente"}' "$(uuid)" | jq -c '{status, end_to_end_id, counterparty}'

step "Bruno issues a one-time QR code (BR Code) and Ana pays it"
BKEY="bruno.$SUFFIX@example.com"
call POST "/v1/accounts/$BRUNO_ACC/pix/keys" "$BRUNO" "{\"type\":\"EMAIL\",\"value\":\"$BKEY\"}" >/dev/null
PAYLOAD=$(call POST "/v1/accounts/$BRUNO_ACC/pix/qr-codes" "$BRUNO" "{\"key\":\"$BKEY\",\"amount\":\"15.00\",\"description\":\"Cafe\",\"one_time\":true,\"expires_in_seconds\":600}" | jq -r .payload)
echo "copia e cola: $PAYLOAD"
call POST /v1/pix/qr-codes/decode "$ANA" "{\"payload\":\"$PAYLOAD\"}" | jq -c '{receiver: .receiver.name, amount, issued_here}'
call POST "/v1/accounts/$ANA_ACC/pix/qr-payments" "$ANA" "{\"payload\":\"$PAYLOAD\"}" "$(uuid)" | jq -c '{status, amount, qr_txid}'
call POST "/v1/accounts/$ANA_ACC/pix/qr-payments" "$ANA" "{\"payload\":\"$PAYLOAD\"}" "$(uuid)" | jq -c '{second_payment: .error.code}'

step "Ana refunds R\$ 20,00 of the Pix she received from Empresa X"
call POST "/v1/accounts/$ANA_ACC/pix/transfers/$PIXIN_ID/refunds" "$ANA" '{"amount":"20.00","reason":"Cobrado a mais"}' "$(uuid)" | jq -c '{status, amount, refund_of, to: .counterparty.name}'

step "Ana gets a virtual debit card and buys at a bakery (simulated card network)"
CARD=$(call POST "/v1/accounts/$ANA_ACC/cards" "$ANA" '{"kind":"VIRTUAL"}' "$(uuid)")
echo "$CARD" | jq -c '{last4, holder_name, expiry: "\(.expiry_month)/\(.expiry_year)", controls}'
CARD_ID=$(echo "$CARD" | jq -r .id)
# Sensitive actions need a fresh password check (step-up).
STEP_UP=$(curl -sS -X POST "$API/v1/auth/step-up" -H "Authorization: Bearer $ANA" -d "{\"password\":\"$PASSWORD\"}" | jq -r .step_up_token)
DETAILS=$(curl -sS -X POST "$API/v1/accounts/$ANA_ACC/cards/$CARD_ID/reveal" -H "Authorization: Bearer $ANA" -H "X-Step-Up-Token: $STEP_UP")
AUTH=$(call POST /v1/sandbox/card-network/authorizations "$ANA" "$(echo "$DETAILS" | jq -c '{pan, expiry_month, expiry_year, cvv, amount: "42.00", merchant_name: "PADARIA SAO JOSE", mcc: "5462", country: "BR", channel: "ONLINE"}')")
echo "$AUTH" | jq -c '{status, response_code, amount}'
WALLET=$(curl -sS -X POST "$API/v1/accounts/$ANA_ACC/cards/$CARD_ID/tokens" -H "Authorization: Bearer $ANA" -H "X-Step-Up-Token: $STEP_UP" -d '{"wallet":"APPLE_PAY","device_name":"iPhone da Ana"}')
echo "$WALLET" | jq -c '{wallet, device_name, token_last4: .last4, status}'
call POST /v1/sandbox/card-network/authorizations "$ANA" "$(jq -nc --arg pan "$(echo "$WALLET" | jq -r .token_pan)" --argjson d "$DETAILS" '{pan: $pan, expiry_month: $d.expiry_month, expiry_year: $d.expiry_year, amount: "7.50", merchant_name: "CAFE DO PONTO", mcc: "5814", country: "BR", channel: "CONTACTLESS"}')" | jq -c '{wallet_purchase: .status, response_code, token_id}'
call POST "/v1/sandbox/card-network/authorizations/$(echo "$AUTH" | jq -r .id)/clear" "$ANA" '{"amount":"40.00"}' | jq -c '{status, cleared_amount}'
call POST /v1/sandbox/card-network/authorizations "$ANA" "$(echo "$DETAILS" | jq -c '{pan, expiry_month, expiry_year, cvv, amount: "10.00", merchant_name: "NYC STORE", mcc: "5311", country: "US", channel: "ONLINE"}')" | jq -c '{international_purchase: .status, response_code, reason}'

step "Ana opens a credit line and buys a TV on credit; the invoice closes and she pays part of it"
call POST "/v1/accounts/$ANA_ACC/credit" "$ANA" '{"closing_day":10}' "$(uuid)" | jq -c '{limit: .limit.amount, available: .available.amount, closing_day}'
CREDIT_AUTH=$(call POST /v1/sandbox/card-network/authorizations "$ANA" "$(echo "$DETAILS" | jq -c '{pan, expiry_month, expiry_year, cvv, amount: "300.00", merchant_name: "LOJA DE TV", mcc: "5732", country: "BR", channel: "ONLINE", function: "CREDIT"}')")
echo "$CREDIT_AUTH" | jq -c '{credit_purchase: .status, function, response_code}'
call POST "/v1/sandbox/card-network/authorizations/$(echo "$CREDIT_AUTH" | jq -r .id)/clear" "$ANA" '{}' >/dev/null
call POST "/v1/sandbox/accounts/$ANA_ACC/credit/cycle" "$ANA" '{"action":"CLOSE"}'
INVOICE_ID=$(call GET "/v1/accounts/$ANA_ACC/credit/invoices" "$ANA" | jq -r '.invoices[1].id')
call POST "/v1/accounts/$ANA_ACC/credit/invoices/$INVOICE_ID/payments" "$ANA" '{"amount":"100.00"}' "$(uuid)" | jq -c '{payment: .status, amount: .amount.amount}'
call GET "/v1/accounts/$ANA_ACC/credit/invoices/$INVOICE_ID" "$ANA" | jq -c '{status, total: .total.amount, paid: .paid.amount, remaining: .remaining.amount, due_date}'

step "Ana pays her electricity bill (boleto), 5 days late: fine and interest are added"
BOLETO=$(call POST /v1/sandbox/boletos "$ANA" "{\"amount\":\"89.90\",\"due_date\":\"$(python3 -c 'import datetime; print(datetime.date.today() - datetime.timedelta(days=5))')\",\"payee_name\":\"CIA PAULISTA DE ENERGIA\"}" | jq -r .formatted_line)
echo "$BOLETO"
call POST /v1/boletos/lookup "$ANA" "{\"line\":\"$BOLETO\"}" | jq -c '{payee: .payee.name, due_date, nominal: .nominal.amount, fine: .fine.amount, interest: .interest.amount, total: .total.amount}'
call POST "/v1/accounts/$ANA_ACC/boleto-payments" "$ANA" "{\"line\":\"$BOLETO\"}" "$(uuid)" | jq -c '{status, amount: .amount.amount}'

step "A Pix the receiving bank rejects: the money comes back"
call POST "/v1/accounts/$ANA_ACC/pix/transfers" "$ANA" '{"key":"recusa@outrobanco.com.br","amount":"10.00"}' "$(uuid)" | jq -c .

step "Trying to spend more than the balance"
call POST "/v1/accounts/$ANA_ACC/transfers" "$ANA" "{\"to_account_id\":\"$BRUNO_ACC\",\"amount\":\"400.00\"}" "$(uuid)" | jq -c .

step "Bruno cannot read Ana's account"
call GET "/v1/accounts/$ANA_ACC/balance" "$BRUNO" | jq -c .

step "Balances"
call GET "/v1/accounts/$ANA_ACC/balance" "$ANA" | jq -c '{ana: .available}'
call GET "/v1/accounts/$BRUNO_ACC/balance" "$BRUNO" | jq -c '{bruno: .available}'

step "Ana's statement (built asynchronously from Kafka events)"
sleep 1
call GET "/v1/accounts/$ANA_ACC/statement" "$ANA" | jq -r '.items[] | "\(.occurred_at[0:19])  \(.direction | .[0:1])  \(.amount.amount | (" " * (8 - length)) + .)  \(.type)  \(.counterparty_name)"'

step "Ana's notifications (built from the events of everything above)"
sleep 1
call GET "/v1/notifications?limit=8" "$ANA" | jq -r '"\(.unread_count) unread", (.notifications[] | "  \(.title): \(.body)")'

printf '\nTraces: http://127.0.0.1:16686  ·  Metrics: http://127.0.0.1:9090\n'
