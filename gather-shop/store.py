"""In-memory order and feedback storage."""

import secrets
from models import OrderStatus


def _gen_id(prefix: str) -> str:
    return f"{prefix}-{secrets.token_hex(3).upper()}"


# --- Orders ---

_orders: dict[str, dict] = {}


def create_cake_order(
    flavor: str,
    size: str,
    toppings: list[str],
    message: str,
    total_bch: str,
    payment_address: str,
) -> dict:
    """Store a new cake order in awaiting_payment state."""
    order_id = _gen_id("ORD")
    order = {
        "order_id": order_id,
        "order_type": "cake",
        "status": OrderStatus.awaiting_payment,
        "flavor": flavor,
        "size": size,
        "toppings": toppings,
        "message": message,
        "total_bch": total_bch,
        "payment_address": payment_address,
        "paid": False,
        "tx_id": None,
    }
    _orders[order_id] = order
    return order


def create_product_order(
    product_id: str,
    product_options: dict[str, str],
    shipping_address: dict,
    total_bch: str,
    payment_address: str,
    design_url: str,
    gelato_product_uid: str,
) -> dict:
    """Store a new product order in awaiting_payment state."""
    order_id = _gen_id("ORD")
    order = {
        "order_id": order_id,
        "order_type": "product",
        "status": OrderStatus.awaiting_payment,
        "product_id": product_id,
        "product_options": product_options,
        "shipping_address": shipping_address,
        "design_url": design_url,
        "gelato_product_uid": gelato_product_uid,
        "total_bch": total_bch,
        "payment_address": payment_address,
        "paid": False,
        "tx_id": None,
        "gelato_order_id": None,
        "tracking_url": None,
    }
    _orders[order_id] = order
    return order


def get_order(order_id: str) -> dict | None:
    return _orders.get(order_id)


def submit_payment(order_id: str, tx_id: str) -> dict | None:
    """Mark an order as paid. Returns updated order or None if not found."""
    order = _orders.get(order_id)
    if order is None:
        return None
    order["tx_id"] = tx_id
    order["paid"] = True
    order["status"] = OrderStatus.confirmed
    return order


def update_gelato_info(order_id: str, gelato_order_id: str) -> dict | None:
    """Store Gelato order ID and update status to fulfilling."""
    order = _orders.get(order_id)
    if order is None:
        return None
    order["gelato_order_id"] = gelato_order_id
    order["status"] = OrderStatus.fulfilling
    return order


def is_tx_used(tx_id: str) -> bool:
    """Check if a transaction ID has already been used for an order."""
    return any(o["tx_id"] == tx_id for o in _orders.values())


# --- Feedback ---

_feedback: list[dict] = []


def create_feedback(rating: int, message: str, agent: str) -> dict:
    fb_id = _gen_id("FB")
    entry = {"feedback_id": fb_id, "rating": rating, "message": message, "agent": agent}
    _feedback.append(entry)
    return entry
