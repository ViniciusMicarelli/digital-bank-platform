#!/usr/bin/env bash
# Seeds a running stack (`make up`) with demo customers that exercise every
# feature, so the web app, the mobile app and the back office have something
# to show. Safe to run again: customers that exist are left as they are.
#
#   Ana Souza    ana@example.com    balance, Pix, debit and credit card, invoice,
#                                    a merchant refund, an open card dispute and
#                                    a savings box with a goal
#   Bruno Lima   bruno@example.com  account blocked by an analyst, with an unblock
#                                    request waiting for a second analyst
#   Carla Dias   carla@example.com  politically exposed: KYC in manual review
set -euo pipefail

API=${API:-http://127.0.0.1:8000}
PASSWORD='correct horse battery staple'
command -v jq >/dev/null || { echo "jq is required (brew install jq)"; exit 1; }
cd "$(dirname "$0")/.."

say() { printf '\033[1m▶ %s\033[0m\n' "$*"; }
uuid() { python3 -c 'import uuid; print(uuid.uuid4())'; }
call() { # call METHOD PATH TOKEN [JSON] [IDEMPOTENCY_KEY]
  local args=(-sS -X "$1" "$API$2" -H "Authorization: Bearer $3" -H 'Content-Type: application/json')
  [[ -n "${4:-}" ]] && args+=(-d "$4")
  [[ -n "${5:-}" ]] && args+=(-H "Idempotency-Key: $5")
  curl "${args[@]}"
}
login() { # login EMAIL [TOTP] -> access token, or empty
  curl -sS -X POST "$API/v1/auth/login" -d "{\"email\":\"$1\",\"password\":\"$PASSWORD\",\"totp_code\":\"${2:-}\"}" | jq -r '.access_token // empty'
}
cpf() { # cpf [PREFIX] -> a valid CPF; prefix 222 is a PEP for the bureau simulator
  python3 -c '
import random, sys
p = sys.argv[1] if len(sys.argv) > 1 else ""
d = [int(c) for c in p] + [random.randint(3, 9)] + [random.randint(0, 9) for _ in range(8 - len(p))]
d = d[:9]
for n in (9, 10): d.append(sum(d[i] * (n + 1 - i) for i in range(n)) * 10 % 11 % 10)
print("".join(map(str, d)))' "${1:-}"
}

curl -fsS "$API/healthz" >/dev/null || { echo "the gateway is not up at $API: run make up"; exit 1; }

# customer EMAIL NAME [CPF_PREFIX] -> token; signs up and verifies identity once.
customer() {
  local token
  token=$(login "$1")
  if [[ -z "$token" ]]; then
    curl -sS -X POST "$API/v1/auth/register" -d "{\"email\":\"$1\",\"password\":\"$PASSWORD\",\"full_name\":\"$2\"}" >/dev/null
    token=$(login "$1")
    call PUT /v1/me/profile "$token" "{\"full_name\":\"$2\",\"cpf\":\"$(cpf "${3:-}")\",\"birth_date\":\"1990-05-20\",\"phone\":\"+5511912345678\",\"address\":{\"postal_code\":\"01310-100\",\"street\":\"Av. Paulista\",\"number\":\"1000\",\"city\":\"São Paulo\",\"state\":\"SP\"}}" >/dev/null
    call POST /v1/me/kyc "$token" >/dev/null
  fi
  echo "$token"
}
account() { # account TOKEN -> the customer's account id (opened if needed)
  local id
  id=$(call GET /v1/accounts "$1" | jq -r '.accounts[0].id // empty')
  [[ -n "$id" ]] || id=$(call POST /v1/accounts "$1" '{}' "$(uuid)" | jq -r .id)
  echo "$id"
}

say "Ana Souza"
ANA=$(customer ana@example.com "Ana Souza")
ANA_ACC=$(account "$ANA")
if [[ $(call GET "/v1/accounts/$ANA_ACC/cards" "$ANA" | jq '.cards | length') == 0 ]]; then
  call POST "/v1/accounts/$ANA_ACC/pix/keys" "$ANA" '{"type":"EMAIL","value":"ana@example.com"}' >/dev/null
  call POST /v1/sandbox/pix-in "$ANA" '{"key":"ana@example.com","amount":"5000.00","payer_name":"Empresa X Ltda"}' >/dev/null
  call POST "/v1/accounts/$ANA_ACC/pix/transfers" "$ANA" '{"key":"maria@outrobanco.com.br","amount":"80.00","description":"Presente"}' "$(uuid)" >/dev/null
  call POST "/v1/accounts/$ANA_ACC/credit" "$ANA" '{"closing_day":10}' "$(uuid)" >/dev/null
  CARD_ID=$(call POST "/v1/accounts/$ANA_ACC/cards" "$ANA" '{"kind":"VIRTUAL"}' "$(uuid)" | jq -r .id)
  STEP_UP=$(curl -sS -X POST "$API/v1/auth/step-up" -H "Authorization: Bearer $ANA" -d "{\"password\":\"$PASSWORD\"}" | jq -r .step_up_token)
  DETAILS=$(curl -sS -X POST "$API/v1/accounts/$ANA_ACC/cards/$CARD_ID/reveal" -H "Authorization: Bearer $ANA" -H "X-Step-Up-Token: $STEP_UP")
  buy() { # buy AMOUNT MERCHANT MCC FUNCTION -> authorization id (cleared)
    local id
    id=$(call POST /v1/sandbox/card-network/authorizations "$ANA" "$(echo "$DETAILS" | jq -c --arg a "$1" --arg m "$2" --arg mcc "$3" --arg f "$4" \
      '{pan, expiry_month, expiry_year, cvv, amount: $a, merchant_name: $m, mcc: $mcc, country: "BR", channel: "ONLINE", function: $f}')" | jq -r .id)
    call POST "/v1/sandbox/card-network/authorizations/$id/clear" "$ANA" '{}' >/dev/null
    echo "$id"
  }
  buy 23.90 "PADARIA SAO JOSE" 5462 DEBIT >/dev/null
  SHOES=$(buy 349.90 "LOJA DE CALCADOS" 5661 DEBIT)
  call POST "/v1/sandbox/card-network/authorizations/$SHOES/refund" "$ANA" '{"amount":"349.90"}' >/dev/null # returned the shoes
  ONLINE=$(buy 189.90 "LOJA ONLINE BRASIL" 5999 DEBIT)
  call POST "/v1/accounts/$ANA_ACC/cards/$CARD_ID/purchases/$ONLINE/disputes" "$ANA" \
    '{"reason":"NOT_RECEIVED","description":"Paguei há 20 dias e o pedido nunca chegou."}' >/dev/null
  buy 899.00 "LOJA DE TV" 5732 CREDIT >/dev/null
  buy 54.90 "STREAMING PLUS" 4899 CREDIT >/dev/null
  BOLETO=$(call POST /v1/sandbox/boletos "$ANA" '{"amount":"189.90","due_date":"2030-01-10","payee_name":"CIA PAULISTA DE ENERGIA"}' | jq -r .formatted_line)
  echo "  boleto para testar o pagamento: $BOLETO"
fi

# A savings box (caixinha) with a goal and two months of yield.
if [[ $(call GET "/v1/accounts/$ANA_ACC/savings" "$ANA" | jq '.boxes | length') == 0 ]]; then
  BOX=$(call POST "/v1/accounts/$ANA_ACC/savings" "$ANA" '{"name":"Reserva de emergência","goal":"10000.00"}' "$(uuid)" | jq -r .id)
  call POST "/v1/accounts/$ANA_ACC/savings/$BOX/deposits" "$ANA" '{"amount":"1500.00"}' "$(uuid)" >/dev/null
  call POST "/v1/sandbox/accounts/$ANA_ACC/savings/$BOX/yield" "$ANA" '{"days":60}' >/dev/null
fi

say "Bruno Lima"
BRUNO=$(customer bruno@example.com "Bruno Lima")
BRUNO_ACC=$(account "$BRUNO")
if [[ $(call GET "/v1/accounts/$BRUNO_ACC" "$BRUNO" | jq -r .status) == ACTIVE ]]; then
  call POST "/v1/accounts/$BRUNO_ACC/pix/keys" "$BRUNO" '{"type":"EMAIL","value":"bruno@example.com"}' >/dev/null
  call POST "/v1/accounts/$ANA_ACC/pix/transfers" "$ANA" '{"key":"bruno@example.com","amount":"150.00","description":"Aluguel"}' "$(uuid)" >/dev/null
  # An analyst blocks the account and asks to unblock it: a second analyst must approve.
  STAFF=$(login analista@digitalbank.dev "$(python3 scripts/totp.py --consume analista@digitalbank.dev)")
  call POST "/v1/backoffice/accounts/$BRUNO_ACC/block" "$STAFF" '{"reason":"Movimentação atípica"}' >/dev/null
  call POST "/v1/backoffice/accounts/$BRUNO_ACC/unblock" "$STAFF" '{"reason":"Cliente confirmou as transações por telefone"}' >/dev/null
fi

say "Carla Dias (PEP: análise manual de KYC)"
customer carla@example.com "Carla Dias" 222 >/dev/null

cat <<EOF

Pronto. Senha de todos: $PASSWORD
  Web           http://localhost:8081   (clientes e back office)
  API           http://localhost:8000
  Clientes      ana@example.com · bruno@example.com · carla@example.com
  Analistas     analista@digitalbank.dev · revisora@digitalbank.dev
                códigos TOTP: make totp
EOF
