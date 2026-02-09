"""Payment verification via Blockchair API.

Queries the BCH blockchain (via Blockchair) to verify that a transaction
exists and sent the correct amount to the shop's payment address.
Accepts 0-conf (mempool) transactions.
"""

import os
import re
from decimal import Decimal

from dotenv import load_dotenv
import httpx

from menu import get_item_price

load_dotenv()

_TX_ID_PATTERN = re.compile(r"^[0-9a-f]{64}$")

BLOCKCHAIR_URL = "https://api.blockchair.com/bitcoin-cash/dashboards/transaction"

# Shop's BCH receiving address. Loaded from .env or environment.
SHOP_BCH_ADDRESS = os.environ.get(
    "SHOP_BCH_ADDRESS",
    "YOUR_BCH_ADDRESS",
)

SATS_PER_BCH = 100_000_000


def _strip_prefix(address: str) -> str:
    """Strip 'bitcoincash:' prefix for comparison with Blockchair recipients."""
    return address.removeprefix("bitcoincash:")


async def verify_transaction(tx_id: str, expected_bch: Decimal) -> tuple[bool, str]:
    """Verify a BCH transaction via Blockchair.

    Checks that:
    1. The transaction exists on the blockchain (or mempool)
    2. At least one output goes to the shop's payment address
    3. That output's value >= expected amount

    Returns (ok, message).
    """
    if not _TX_ID_PATTERN.match(tx_id):
        return False, (
            f"Invalid transaction ID format. Expected a 64-character lowercase hex string. "
            f"Received: '{tx_id}' ({len(tx_id)} chars). "
            f"A valid tx_id looks like: b1e4b40416eb4f471ed66ee7c5fd5679cee39f38b7240660ad5e0db6bd854528"
        )

    expected_sats = int(expected_bch * SATS_PER_BCH)
    shop_addr = _strip_prefix(SHOP_BCH_ADDRESS)

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.get(f"{BLOCKCHAIR_URL}/{tx_id}")
    except httpx.HTTPError:
        return False, "Payment verification service unavailable. Please try again."

    if resp.status_code != 200:
        return False, "Payment verification service returned an error. Please try again."

    data = resp.json().get("data", {})

    # Blockchair returns data as {tx_hash: {...}} or empty dict/list if not found
    tx_data = data.get(tx_id) if isinstance(data, dict) else None
    if not tx_data:
        return False, f"Transaction {tx_id} not found on the BCH blockchain."

    outputs = tx_data.get("outputs", [])
    for output in outputs:
        if output.get("recipient") == shop_addr:
            if output.get("value", 0) >= expected_sats:
                return True, "Payment verified on blockchain."
            else:
                actual_bch = Decimal(output["value"]) / SATS_PER_BCH
                return False, (
                    f"Payment amount insufficient. "
                    f"Expected >= {expected_bch} BCH, found {actual_bch} BCH."
                )

    return False, (
        f"Transaction does not include payment to shop address ({SHOP_BCH_ADDRESS}). "
        f"Please send BCH to the payment_address returned in your order."
    )


def calculate_total(
    flavor: str, size: str, toppings: list[str]
) -> tuple[Decimal, None] | tuple[None, list[str]]:
    """Calculate order total from selected items.

    Returns (total, None) on success, or (None, list_of_invalid_ids) on failure.
    """
    invalid: list[str] = []

    flavor_price = get_item_price("flavors", flavor)
    if flavor_price is None:
        invalid.append(f"flavor '{flavor}' (see GET /menu/flavors)")

    size_price = get_item_price("sizes", size)
    if size_price is None:
        invalid.append(f"size '{size}' (see GET /menu/sizes)")

    bad_toppings = [t for t in toppings if get_item_price("toppings", t) is None]
    if bad_toppings:
        invalid.append(f"toppings {bad_toppings} (see GET /menu/toppings)")

    if invalid:
        return None, invalid

    total = Decimal(flavor_price) + Decimal(size_price)
    for topping_id in toppings:
        total += Decimal(get_item_price("toppings", topping_id))

    return total, None
