# notification-service

## Responsibility

Delivers notifications to customers through **push**, **email** and **SMS**, triggered by domain events.

## Capabilities

- Templating per event type and channel (with localization: `pt-BR`, `en-US`)
- Customer preferences (opt-in / opt-out per channel and category)
- Provider abstraction (ports) with simulated adapters for local development
- Retries with backoff and fallback channel (push fails → SMS for critical messages)
- Delivery status tracking
- Rate limiting to avoid spamming users

## Examples

| Event | Channel | Message |
|---|---|---|
| `pix.transfer.received` | push | "You received R$ 50.00 from Alice" |
| `card.authorization.approved` | push | "Purchase approved: R$ 32.90 at Coffee Shop" |
| `credit.invoice.closed` | push, email | "Your invoice closed: R$ 1,234.56, due on 10/02" |
| `identity.device.registered` | push, email | "New device signed in to your account" |

## gRPC API

| Method | Description |
|---|---|
| `GetPreferences` / `UpdatePreferences` | Notification settings |
| `ListNotifications` | Notification inbox in the app |
| `RegisterPushToken` | Register a device push token |

## Events

| Publishes | Consumes |
|---|---|
| `notification.sent` | Most domain events (see [event catalog](../events/README.md)) |
| `notification.failed` | |

## Data

`notifications`, `preferences`, `templates`, `push_tokens`, `inbox`.
