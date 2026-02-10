# Plan: Real BCH Payment Verification

## Goal
Replace the simulated payment stub with real on-chain Bitcoin Cash transaction verification via the Blockchair API. An agent should be able to discover the shop's BCH address, calculate the price, send payment, and have the shop verify it — all from the API alone.

## What Changes

### 1. Shop BCH address — configurable, exposed in the API
- Add a `SHOP_BCH_ADDRESS` env var (with a hardcoded default for the demo)
- Expose it in the root discovery response (`GET /`) so agents know where to send payment
- Also return it in a new `POST /order/preview` response

### 2. New endpoint: `POST /order/preview`
User-tester flagged that agents can't calculate the total before ordering. This endpoint takes the same item selection as `/order` (flavor, size, toppings) but returns the total price and the shop's payment address — no payment required yet.

```
POST /order/preview
{ "flavor": "chocolate", "size": "medium", "toppings": ["sprinkles"] }
→ { "total_bch": "0.016", "pay_to": "bitcoincash:qr...", "valid_for_seconds": 300 }
```

This also validates the item selection, so the agent gets errors before attempting payment.

### 3. Real transaction verification via Blockchair
Replace `payment.py` internals. On `POST /order`:

1. Call `https://api.blockchair.com/bitcoin-cash/dashboards/transaction/{tx_id}`
2. Verify the tx exists (`context.results > 0`)
3. Check that at least one output pays the shop's address
4. Check the output value >= expected total (values are in satoshis, so convert BCH decimal → satoshis)
5. Optionally check confirmations (`context.state - transaction.block_id + 1`), but accept 0-conf for demo speed

**Blockchair free tier:** 30 req/min, 1440/day — plenty for a demo.

**Response structure (confirmed from live API):**
- `data.{hash}.transaction.block_id` — block number, or `-1` if unconfirmed
- `data.{hash}.outputs[].recipient` — receiving address (cashaddr without prefix)
- `data.{hash}.outputs[].value` — amount in satoshis
- `context.state` — current block height

### 4. BCH address format handling
Blockchair returns addresses in short cashaddr format (no `bitcoincash:` prefix). We need to normalize when comparing — strip the prefix from the shop address before matching against tx outputs.

### 5. Duplicate tx_id prevention
Already implemented in the quick fixes (store.py `is_tx_used`). No change needed.

## Files Changed

| File | Change |
|------|--------|
| `payment.py` | Replace stub with real Blockchair API call + validation logic |
| `models.py` | Add `OrderPreviewRequest`, `OrderPreviewResponse` models; add `pay_to` to `DiscoveryResponse` |
| `main.py` | Add `/order/preview` endpoint; pass shop address config; add `httpx` for async HTTP |
| `store.py` | No changes |
| `menu.py` | No changes |
| `requirements.txt` | Add `httpx` for async HTTP client |

## Dependencies
- `httpx` — async HTTP client for calling Blockchair (FastAPI is async, so we should use an async client)

## What We Need From You
- **A real BCH address for the shop.** I can use a placeholder for now, but for a live demo you'll want your own. Set it via `SHOP_BCH_ADDRESS` env var.

## What This Does NOT Do
- No wallet management / key generation on the server side
- No refund logic
- No confirmation-waiting / webhook — we check on-chain status at order time
- No price volatility handling (BCH prices are fixed in the menu)
