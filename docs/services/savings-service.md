# savings-service

## Responsibility

Savings boxes (*caixinhas*): money a customer sets apart from the checking account, optionally towards a goal, that yields every day and can be withdrawn at any time. Each box is its **own customer account in the ledger** (the box id is the ledger account id), so the money stays the customer's, the balance always comes from the ledger, and a box can never go negative.

## Capabilities

| Capability | Status |
|---|---|
| Up to 20 active boxes per checking account, each with a name and an optional goal; rename, change the goal, close (everything goes back to the checking account) | ✅ |
| Deposit and withdraw, idempotent on the client's key: one-step `SAVINGS_DEPOSIT` / `SAVINGS_WITHDRAWAL` in transaction-service (own money: no transfer limits; the checking account must be active); a refusal (`INSUFFICIENT_FUNDS`, from the ledger's floor) is a `FAILED` movement, not an error | ✅ |
| Daily yield at a yearly rate (`SAVINGS_ANNUAL_RATE_BP`, default 1065 = 10.65%), compounded per calendar day on the balance at the end of each Brasília day, from the day after the box is created; paid from the ledger's internal `savings-yield` account (`SAVINGS_YIELD`) | ✅ |
| No rounding loss: the fraction of a centavo is carried (in millionths) to the next day | ✅ |
| Exactly once: each day's yield has a deterministic key (`yield:<day>`); a crash or outage between paying and recording replays the same movement; the loop catches up on missed days | ✅ |
| Movements left `PENDING` by an outage are settled by the recovery loop | ✅ |
| Monthly scheduled deposit (*depósito programado*): an amount on a day 1–28, first run the next such day (setting it never moves money); runs once per month with a deterministic key (`schedule:<day>`), an outage keeps it due, a refusal for lack of funds skips the month and is kept on the box (`last_failure`); months missed while the service was down are not caught up | ✅ |
| Sandbox: pay N days of yield at once on today's balance; make the next monthly deposit now | ✅ |
| Business-day CDI with the daily published rate, income tax (IOF / IR regressivo) | planned |

## Money flow

```mermaid
flowchart LR
    ACC[checking account] -- SAVINGS_DEPOSIT --> BOX[box ledger account]
    BOX -- SAVINGS_WITHDRAWAL --> ACC
    Y[(savings-yield<br/>internal 0007)] -- SAVINGS_YIELD daily --> BOX
```

## gRPC

| Method | Description |
|---|---|
| `CreateBox` / `GetBox` / `ListBoxes` / `UpdateBox` / `CloseBox` | Boxes |
| `Deposit` / `Withdraw` / `ListMovements` | Money in and out, history |
| `SetSchedule` | Set or (zero amount) remove the monthly deposit |
| `SimulateYield` / `RunSchedule` | Sandbox |

## Configuration

| Variable | Default |
|---|---|
| `GRPC_ADDR` | `:9014` |
| `METRICS_ADDR` | `:9114` |
| `DATABASE_URL` | `postgres://bank:bank@localhost:54329/savings` |
| `LEDGER_ADDR` / `TRANSACTION_ADDR` | `127.0.0.1:9001` / `127.0.0.1:9003` |
| `SAVINGS_ANNUAL_RATE_BP` | `1065` |
| `WORKER_INTERVAL` | `30s` (yield, monthly deposits and recovery) |
