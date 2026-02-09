"""Pydantic models — the contract between agent and cake shop."""

from pydantic import BaseModel, Field
from enum import Enum


# --- Menu Models ---

class CategoryInfo(BaseModel):
    """A browsable menu category with a link to its items."""
    id: str = Field(description="Category identifier used in URLs", examples=["flavors"])
    name: str = Field(description="Human/agent-readable category name", examples=["Cake Flavors"])
    count: int = Field(description="Number of items in this category", examples=[5])
    href: str = Field(description="URL to browse items in this category", examples=["/menu/flavors"])


class MenuResponse(BaseModel):
    """Top-level menu: categories the agent can browse into."""
    categories: list[CategoryInfo]


class MenuItem(BaseModel):
    """A single purchasable item within a category."""
    id: str = Field(description="Item identifier used when ordering", examples=["chocolate"])
    name: str = Field(description="Display name", examples=["Chocolate Fudge"])
    available: bool = Field(description="Whether this item can be ordered right now")
    base_price_bch: str = Field(
        description="Price in BCH as a decimal string",
        examples=["0.010"],
    )


class CategoryResponse(BaseModel):
    """Paginated list of items within a single category."""
    category: str = Field(description="Category ID", examples=["flavors"])
    items: list[MenuItem]
    page: int = Field(description="Current page number (1-indexed)", examples=[1])
    total_pages: int = Field(description="Total number of pages", examples=[1])
    next: str | None = Field(
        description="URL for the next page, or null if this is the last page",
        examples=["/menu/flavors?page=2"],
    )


# --- Order Models ---

class OrderRequest(BaseModel):
    """Place a cake order. Payment details are returned in the response."""
    flavor: str = Field(description="Flavor ID from /menu/flavors", examples=["chocolate"])
    size: str = Field(description="Size ID from /menu/sizes", examples=["medium"])
    toppings: list[str] = Field(
        default=[],
        description="List of topping IDs from /menu/toppings",
        examples=[["sprinkles", "caramel_drizzle"]],
    )
    message: str = Field(
        default="",
        description="Optional message to write on the cake",
        examples=["Happy Birthday!"],
    )


class OrderStatus(str, Enum):
    awaiting_payment = "awaiting_payment"
    confirmed = "confirmed"
    fulfilling = "fulfilling"
    shipped = "shipped"
    baking = "baking"
    ready = "ready"


class OrderResponse(BaseModel):
    """Confirmation returned after placing an order. Includes payment instructions."""
    order_id: str = Field(description="Unique order identifier", examples=["ORD-1A2B3C"])
    status: OrderStatus = Field(description="Current order status")
    total_bch: str = Field(description="Total price to pay in BCH", examples=["0.018"])
    payment_address: str = Field(
        description="BCH address to send payment to",
        examples=["bitcoincash:qr2z7dusk64k7sx0gq5xdexp3lmqnkpmc5nq0pyar"],
    )
    status_url: str = Field(
        description="URL to check order status and submit payment",
        examples=["/order/ORD-1A2B3C"],
    )


class OrderStatusResponse(BaseModel):
    """Current status of an existing order, including payment details."""
    order_id: str
    status: OrderStatus
    order_type: str = Field(description="'cake' or 'product'", examples=["cake"])
    total_bch: str
    payment_address: str = Field(description="BCH address to send payment to")
    paid: bool = Field(description="Whether payment has been verified")
    tx_id: str | None = Field(description="Transaction ID if payment submitted, null otherwise")
    # Cake-specific fields (present when order_type == "cake")
    flavor: str | None = Field(default=None, description="Cake flavor (cake orders only)")
    size: str | None = Field(default=None, description="Cake size (cake orders only)")
    toppings: list[str] | None = Field(default=None, description="Cake toppings (cake orders only)")
    message: str | None = Field(default=None, description="Cake message (cake orders only)")
    # Product-specific fields (present when order_type == "product")
    product_id: str | None = Field(default=None, description="Product ID (product orders only)")
    product_options: dict[str, str] | None = Field(default=None, description="Chosen options like size/color (product orders only)")
    gelato_order_id: str | None = Field(default=None, description="Gelato fulfillment order ID (product orders only)")
    tracking_url: str | None = Field(default=None, description="Shipping tracking URL when available (product orders only)")


class ShippingAddress(BaseModel):
    """Shipping address for physical product orders."""
    first_name: str = Field(description="Recipient first name", examples=["Alice"])
    last_name: str = Field(description="Recipient last name", examples=["Smith"])
    address_line_1: str = Field(description="Street address", examples=["123 Main St"])
    address_line_2: str = Field(default="", description="Apt, suite, etc.", examples=["Apt 4B"])
    city: str = Field(description="City", examples=["Portland"])
    state: str = Field(default="", description="State/province (required for US)", examples=["OR"])
    post_code: str = Field(description="Postal/ZIP code", examples=["97201"])
    country: str = Field(description="ISO 2-letter country code", examples=["US"])
    email: str = Field(description="Contact email for shipping updates", examples=["alice@example.com"])
    phone: str = Field(default="", description="Contact phone number", examples=["5551234567"])


class ProductOrderRequest(BaseModel):
    """Order a real, shippable product. Fulfilled via Gelato print-on-demand."""
    product_id: str = Field(
        description="Product ID from /menu/products",
        examples=["t-shirt"],
    )
    options: dict[str, str] = Field(
        description="Product options (size, color, etc.). See GET /products/{id}/options for valid values.",
        examples=[{"size": "m", "color": "white"}],
    )
    shipping_address: ShippingAddress


class ProductOptionsResponse(BaseModel):
    """Available options for a product (sizes, colors, etc.)."""
    product_id: str
    product_name: str
    options: dict[str, list[str]] = Field(
        description="Available values for each option",
        examples=[{"size": ["s", "m", "l", "xl"], "color": ["white", "black"]}],
    )


class PaymentRequest(BaseModel):
    """Submit a BCH transaction ID to pay for an order."""
    tx_id: str = Field(
        description="BCH transaction ID (64-char hex hash) proving payment was sent to the payment_address",
        examples=["b1e4b40416eb4f471ed66ee7c5fd5679cee39f38b7240660ad5e0db6bd854528"],
    )


class PaymentResponse(BaseModel):
    """Result of payment verification."""
    order_id: str = Field(description="Order that was paid")
    status: OrderStatus = Field(description="Updated order status")
    tx_id: str = Field(description="Verified transaction ID")
    total_bch: str = Field(description="Amount verified")


# --- Feedback Models ---

class FeedbackRequest(BaseModel):
    """Agent feedback on the ordering experience. No auth required."""
    rating: int = Field(
        description="1-5 star rating of the experience",
        ge=1, le=5,
        examples=[5],
    )
    message: str = Field(
        default="",
        description="Optional free-text feedback",
        examples=["Clear instructions, smooth transaction."],
    )
    agent: str = Field(
        default="",
        description="Optional: which agent/model submitted this feedback",
        examples=["claude-code"],
    )


class FeedbackResponse(BaseModel):
    """Acknowledgement of submitted feedback."""
    status: str = Field(default="thanks", examples=["thanks"])
    feedback_id: str = Field(description="ID for this feedback entry", examples=["FB-001"])


# --- Help Models ---

class SetupStep(BaseModel):
    """One step in setting up a prerequisite."""
    action: str = Field(description="What to do")
    code: str | None = Field(default=None, description="Code to execute, if applicable")
    note: str | None = Field(default=None, description="Additional context")


class Prerequisite(BaseModel):
    """Something the agent needs before it can transact."""
    id: str = Field(description="Prerequisite identifier")
    name: str = Field(description="Human-readable name")
    why: str = Field(description="Why this is needed")
    setup: list[SetupStep] = Field(description="Actionable steps to fulfill this prerequisite")
    check: str = Field(description="How to verify this prerequisite is met")


class WorkflowStep(BaseModel):
    """One step in the end-to-end transaction workflow."""
    step: int
    action: str
    endpoint: str | None = Field(default=None, description="API endpoint for this step, if applicable")
    detail: str


class EndpointHelp(BaseModel):
    """Usage tips for a specific endpoint."""
    method: str
    path: str
    purpose: str
    tips: list[str]


class HelpResponse(BaseModel):
    """Complete guide for an AI agent to set up and use this API."""
    overview: str = Field(description="What this API does and what you need to use it")
    prerequisites: list[Prerequisite]
    workflow: list[WorkflowStep]
    endpoints: list[EndpointHelp]
