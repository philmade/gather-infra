"""Gelato print-on-demand API client.

Places real orders with Gelato after BCH payment is verified.
Requires GELATO_API_KEY in .env or environment.
"""

import os
from dotenv import load_dotenv
import httpx

load_dotenv()

GELATO_API_KEY = os.environ.get("GELATO_API_KEY", "")
GELATO_ORDERS_URL = "https://order.gelatoapis.com/v4/orders"


async def place_gelato_order(
    product_uid: str,
    design_url: str,
    shipping: dict,
    our_order_id: str,
) -> tuple[str | None, str]:
    """Place an order with Gelato.

    Args:
        product_uid: Resolved Gelato product UID
        design_url: Public URL to the design image
        shipping: Dict with firstName, lastName, addressLine1, city, postCode, country, email, phone
        our_order_id: Our internal order ID (used as orderReferenceId)

    Returns:
        (gelato_order_id, message) on success, or (None, error_message) on failure.
    """
    if not GELATO_API_KEY:
        return None, (
            "Gelato API key not configured. "
            "Set GELATO_API_KEY in .env to enable real order fulfillment."
        )

    payload = {
        "orderType": "order",
        "orderReferenceId": our_order_id,
        "customerReferenceId": our_order_id,
        "currency": "USD",
        "items": [
            {
                "itemReferenceId": f"{our_order_id}-item-1",
                "productUid": product_uid,
                "files": [
                    {"type": "default", "url": design_url},
                ],
                "quantity": 1,
            },
        ],
        "shippingAddress": shipping,
    }

    try:
        async with httpx.AsyncClient(timeout=15.0) as client:
            resp = await client.post(
                GELATO_ORDERS_URL,
                json=payload,
                headers={"X-API-KEY": GELATO_API_KEY},
            )
    except httpx.HTTPError:
        return None, "Gelato API unavailable. Order saved — fulfillment will be retried."

    if resp.status_code in (200, 201):
        data = resp.json()
        gelato_id = data.get("id", data.get("orderId", "unknown"))
        return str(gelato_id), "Order placed with Gelato for fulfillment."

    return None, f"Gelato API error ({resp.status_code}): {resp.text[:200]}"


async def get_gelato_order_status(gelato_order_id: str) -> str | None:
    """Check order status with Gelato. Returns status string or None on error."""
    if not GELATO_API_KEY:
        return None

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.get(
                f"https://order.gelatoapis.com/v4/orders/{gelato_order_id}",
                headers={"X-API-KEY": GELATO_API_KEY},
            )
    except httpx.HTTPError:
        return None

    if resp.status_code == 200:
        return resp.json().get("status")
    return None
