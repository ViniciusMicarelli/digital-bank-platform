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

step "They open accounts"
ANA_ACC=$(call POST /v1/accounts "$ANA" '{"holder_name":"Ana Souza","document":"529.982.247-25"}' "$(uuid)" | tee /dev/stderr | jq -r .id)
BRUNO_ACC=$(call POST /v1/accounts "$BRUNO" '{"holder_name":"Bruno Lima","document":"111.444.777-35"}' "$(uuid)" | jq -r .id)

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
DETAILS=$(call POST "/v1/accounts/$ANA_ACC/cards/$CARD_ID/reveal" "$ANA")
AUTH=$(call POST /v1/sandbox/card-network/authorizations "$ANA" "$(echo "$DETAILS" | jq -c '{pan, expiry_month, expiry_year, cvv, amount: "42.00", merchant_name: "PADARIA SAO JOSE", mcc: "5462", country: "BR", channel: "ONLINE"}')")
echo "$AUTH" | jq -c '{status, response_code, amount}'
call POST "/v1/sandbox/card-network/authorizations/$(echo "$AUTH" | jq -r .id)/clear" "$ANA" '{"amount":"40.00"}' | jq -c '{status, cleared_amount}'
call POST /v1/sandbox/card-network/authorizations "$ANA" "$(echo "$DETAILS" | jq -c '{pan, expiry_month, expiry_year, cvv, amount: "10.00", merchant_name: "NYC STORE", mcc: "5311", country: "US", channel: "ONLINE"}')" | jq -c '{international_purchase: .status, response_code, reason}'

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

printf '\nTraces: http://127.0.0.1:16686  ·  Metrics: http://127.0.0.1:9090\n'
