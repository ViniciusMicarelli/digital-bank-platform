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
token() { curl -sS -X POST "$API/v1/auth/token" -d "{\"customer_id\":\"$1\"}" | jq -r .access_token; }
uuid() { python3 -c 'import uuid; print(uuid.uuid4())'; }

SUFFIX=$(uuid | cut -c1-8)
ANA=$(token "ana-$SUFFIX")
BRUNO=$(token "bruno-$SUFFIX")

step "Ana and Bruno open accounts"
ANA_ACC=$(call POST /v1/accounts "$ANA" '{"holder_name":"Ana Souza","document":"529.982.247-25"}' "$(uuid)" | tee /dev/stderr | jq -r .id)
BRUNO_ACC=$(call POST /v1/accounts "$BRUNO" '{"holder_name":"Bruno Lima","document":"111.444.777-35"}' "$(uuid)" | jq -r .id)

step "Ana registers a Pix key and receives R\$ 500,00 from another bank (sandbox)"
KEY="ana.$SUFFIX@example.com"
call POST "/v1/accounts/$ANA_ACC/pix/keys" "$ANA" "{\"type\":\"EMAIL\",\"value\":\"$KEY\"}" | jq -c .
call POST /v1/sandbox/pix-in "$ANA" "{\"key\":\"$KEY\",\"amount\":\"500.00\",\"payer_name\":\"Empresa X\"}" | jq -c '{status, amount, end_to_end_id}'

step "Ana transfers R\$ 120,00 to Bruno (sent twice with the same Idempotency-Key)"
IDEM=$(uuid)
for _ in 1 2; do
  call POST "/v1/accounts/$ANA_ACC/transfers" "$ANA" "{\"to_account_id\":\"$BRUNO_ACC\",\"amount\":\"120.00\",\"description\":\"Aluguel\"}" "$IDEM" | jq -c '{id, status, amount}'
done

step "Ana looks up a key at another bank and sends a Pix"
call GET /v1/pix/keys/maria@outrobanco.com.br "$ANA" | jq -c .
call POST "/v1/accounts/$ANA_ACC/pix/transfers" "$ANA" '{"key":"maria@outrobanco.com.br","amount":"80.00","description":"Presente"}' "$(uuid)" | jq -c '{status, end_to_end_id, counterparty}'

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
