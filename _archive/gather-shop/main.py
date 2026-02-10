"""
Gather Skill Demo — AI-Ready Shop

A FastAPI service that any AI agent can discover, understand, and transact with.
Discovery is handled entirely by FastAPI's auto-generated OpenAPI docs.
Browse /docs for the interactive Swagger UI, or fetch /openapi.json for the raw spec.

## Agent Flow
1. GET /help — complete guide (start here)
2. GET /menu — browse categories (cakes + real shippable products)
3. GET /menu/{category} — browse items (paginated)
4. POST /order — create a cake order | POST /order/product — order a real product
5. PUT /order/{id}/payment — submit BCH transaction ID (verified via blockchain)
6. GET /order/{id} — check order status (product orders show fulfillment tracking)
7. POST /feedback — tell us how the experience was
"""

import math
from decimal import Decimal

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import RedirectResponse

from models import (
    CategoryInfo,
    CategoryResponse,
    EndpointHelp,
    FeedbackRequest,
    FeedbackResponse,
    HelpResponse,
    MenuResponse,
    OrderRequest,
    OrderResponse,
    OrderStatusResponse,
    PaymentRequest,
    PaymentResponse,
    Prerequisite,
    ProductOptionsResponse,
    ProductOrderRequest,
    SetupStep,
    WorkflowStep,
)
from menu import get_cake_categories, get_category_items, is_cake_category, ITEMS_PER_PAGE
from payment import verify_transaction, calculate_total, SHOP_BCH_ADDRESS
import products
from gelato import place_gelato_order
from store import (
    create_cake_order,
    create_product_order,
    get_order,
    create_feedback,
    is_tx_used,
    submit_payment,
    update_gelato_info,
)

app = FastAPI(
    title="Gather Skill Demo — AI-Ready Shop",
    description=(
        "An API designed to be discovered and used by AI agents. "
        "Browse custom cakes (demo) and **real shippable products** "
        "(t-shirts, mugs, framed prints — fulfilled by Gelato print-on-demand). "
        "Product options and BCH prices are live from Gelato's catalog. "
        "Pay with Bitcoin Cash (BCH), verified on the real blockchain. "
        "**Start at GET /help** for the complete agent guide — prerequisites "
        "(wallet setup), step-by-step workflow, and per-endpoint tips."
    ),
    version="3.0.0",
)


# --- Root ---


@app.get("/", include_in_schema=False)
async def root():
    """Redirect to the help guide."""
    return RedirectResponse(url="/help")


# --- Help ---


@app.get("/help", response_model=HelpResponse, tags=["Help"])
async def get_help():
    """Complete guide for an AI agent to set up and use this API.

    Call this first. Returns prerequisites (how to get a BCH wallet),
    the full transaction workflow, and per-endpoint usage tips.
    """
    return HelpResponse(
        overview=(
            "This shop sells custom cakes (demo) and real shippable products "
            "(t-shirts, mugs, framed prints) fulfilled by Gelato print-on-demand. "
            "All payments are in Bitcoin Cash (BCH), verified on the real blockchain. "
            "Product options (sizes, colors) and BCH prices are live — they reflect "
            "Gelato's current catalog and the real-time BCH exchange rate. "
            "To place an order, you need: (1) a BCH wallet with funds, "
            "(2) browse the menu to choose items, (3) create an order, "
            "(4) send BCH to the payment address, (5) submit your transaction ID. "
            "IMPORTANT FOR AUTONOMOUS AGENTS: You cannot fund a wallet yourself. "
            "If you do not already have a funded BCH wallet, you must report back "
            "to your human operator and request they send BCH to your wallet address. "
            "Think of it as asking for pocket money before you go to the shops."
        ),
        prerequisites=[
            Prerequisite(
                id="bch_wallet",
                name="BCH Wallet",
                why=(
                    "Orders are paid in Bitcoin Cash. You need a wallet that can "
                    "send BCH and give you the transaction ID."
                ),
                setup=[
                    SetupStep(
                        action="Install the bitcash Python library",
                        code="pip install bitcash",
                    ),
                    SetupStep(
                        action="Create a new wallet (private key)",
                        code="from bitcash import Key\nkey = Key()\nprint(key.address)  # your bitcoincash: address",
                        note=(
                            "IMPORTANT: Save your key's secret (key.to_wif()) securely. "
                            "If you lose it, you lose access to any BCH in that wallet."
                        ),
                    ),
                    SetupStep(
                        action="Fund your wallet",
                        note=(
                            "YOU CANNOT DO THIS YOURSELF. As an AI agent, you cannot purchase "
                            "or generate BCH. You must ask your human operator to send BCH to "
                            "your wallet address. Present them with your address and the amount "
                            "needed (typical orders cost 0.01-0.05 BCH). Wait for them "
                            "to confirm the transfer before proceeding."
                        ),
                    ),
                    SetupStep(
                        action="Check your balance",
                        code="key.get_balance('bch')  # returns balance as string",
                    ),
                    SetupStep(
                        action="Send a payment (used after creating an order)",
                        code=(
                            "# PAYMENT_ADDRESS and AMOUNT come from the order response\n"
                            "tx_id = key.send([(PAYMENT_ADDRESS, AMOUNT, 'bch')])\n"
                            "print(tx_id)  # this is the transaction ID for PUT /order/{id}/payment"
                        ),
                    ),
                ],
                check="key.get_balance('bch') returns a non-zero balance and key.address starts with 'bitcoincash:'",
            ),
            Prerequisite(
                id="shipping_address",
                name="Shipping Address (for real products only)",
                why=(
                    "Real products (t-shirts, mugs, prints) are physically shipped. "
                    "You need a shipping address from your human operator."
                ),
                setup=[
                    SetupStep(
                        action="Ask your human operator for a shipping address",
                        note=(
                            "You need: first_name, last_name, address_line_1, city, "
                            "post_code, country (ISO 2-letter code like 'US' or 'GB'), "
                            "and email. This is NOT needed for cake orders (demo only)."
                        ),
                    ),
                ],
                check="You have all required address fields to include in POST /order/product.",
            ),
        ],
        workflow=[
            WorkflowStep(
                step=1,
                action="Read this help guide",
                endpoint="GET /help",
                detail="Understand prerequisites and the full flow before starting.",
            ),
            WorkflowStep(
                step=2,
                action="Set up a BCH wallet",
                detail="Follow the prerequisite steps above. You only need to do this once.",
            ),
            WorkflowStep(
                step=3,
                action="Get your wallet funded by your human operator",
                detail=(
                    "STOP HERE IF YOUR WALLET IS EMPTY. You cannot generate or acquire BCH "
                    "on your own. Report back to your human operator with: (a) your wallet "
                    "address, (b) the approximate amount needed (check the menu first if you "
                    "want to be precise). Ask them to send BCH to your address and confirm "
                    "when the transfer is complete. Do not proceed until you have funds."
                ),
            ),
            WorkflowStep(
                step=4,
                action="Browse the menu",
                endpoint="GET /menu",
                detail=(
                    "Returns all categories including cake options and real shippable products. "
                    "The 'products' category contains real items that will be printed and shipped. "
                    "Product prices are live — they reflect Gelato's current cost + our margin, "
                    "converted to BCH at the real-time exchange rate."
                ),
            ),
            WorkflowStep(
                step=5,
                action="Browse items and check product options",
                endpoint="GET /menu/{category}",
                detail=(
                    "Get item IDs and prices. For real products, also call "
                    "GET /products/{id}/options to see available sizes, colors, etc. "
                    "These options come live from Gelato's catalog."
                ),
            ),
            WorkflowStep(
                step=6,
                action="Create an order",
                detail=(
                    "For cakes: POST /order with flavor, size, toppings. "
                    "For real products: POST /order/product with product_id, options, "
                    "and shipping_address. Both return order_id, total_bch, and payment_address."
                ),
            ),
            WorkflowStep(
                step=7,
                action="Send BCH payment",
                detail=(
                    "Using your wallet, send exactly total_bch to the payment_address. "
                    "Save the transaction ID returned by your wallet."
                ),
            ),
            WorkflowStep(
                step=8,
                action="Submit transaction ID",
                endpoint="PUT /order/{order_id}/payment",
                detail=(
                    "Send your tx_id. The server verifies it on the BCH blockchain. "
                    "For product orders, a real fulfillment order is placed with Gelato "
                    "and the item will be printed and shipped to your address."
                ),
            ),
            WorkflowStep(
                step=9,
                action="Check order status",
                endpoint="GET /order/{order_id}",
                detail=(
                    "Track your order. Product orders progress: "
                    "awaiting_payment → confirmed → fulfilling → shipped."
                ),
            ),
            WorkflowStep(
                step=10,
                action="Leave feedback (optional)",
                endpoint="POST /feedback",
                detail="No auth needed. Tell us if the flow was easy or where you got stuck.",
            ),
        ],
        endpoints=[
            EndpointHelp(
                method="GET",
                path="/help",
                purpose="This guide. Call first.",
                tips=["Returns structured JSON, not prose. Parse it programmatically."],
            ),
            EndpointHelp(
                method="GET",
                path="/menu",
                purpose="Top-level category listing",
                tips=[
                    "Follow the 'href' in each category to get items.",
                    "The 'products' category contains real shippable items.",
                    "Product prices are live (Gelato cost + margin → BCH at current rate).",
                    "Don't hardcode category names — always read from this response.",
                ],
            ),
            EndpointHelp(
                method="GET",
                path="/menu/{category}",
                purpose="Paginated items within a category",
                tips=[
                    "Use 'next' field to paginate. null means last page.",
                    "Item 'id' values are what you pass to the order endpoints.",
                    "For cakes: base_price_bch is per-item. Total = flavor + size + sum(toppings).",
                    "For products: base_price_bch is the full price for that product.",
                ],
            ),
            EndpointHelp(
                method="GET",
                path="/products/{product_id}/options",
                purpose="Get available options (sizes, colors) for a shippable product",
                tips=[
                    "Only applies to items from /menu/products.",
                    "Options come live from Gelato's catalog — values may change over time.",
                    "You must include all required options when ordering.",
                ],
            ),
            EndpointHelp(
                method="POST",
                path="/order",
                purpose="Create a cake order (demo)",
                tips=[
                    "Only flavor and size are required. Toppings and message are optional.",
                    "Response includes payment_address and total_bch — you need both to pay.",
                ],
            ),
            EndpointHelp(
                method="POST",
                path="/order/product",
                purpose="Order a real, shippable product (t-shirt, mug, print)",
                tips=[
                    "Requires product_id, options, and shipping_address.",
                    "Get valid options from GET /products/{id}/options first.",
                    "Shipping address must include first_name, last_name, address_line_1, city, post_code, country, email.",
                    "After payment, a real order is placed with Gelato for printing and shipping.",
                    "The BCH price is calculated live at order time — it may differ slightly from the menu listing.",
                ],
            ),
            EndpointHelp(
                method="PUT",
                path="/order/{order_id}/payment",
                purpose="Submit BCH transaction ID to complete payment",
                tips=[
                    "tx_id must be a valid BCH transaction hash (64 hex characters).",
                    "The transaction must have an output to payment_address for >= total_bch.",
                    "0-conf accepted — no need to wait for block confirmations.",
                    "For product orders, payment triggers real fulfillment via Gelato.",
                ],
            ),
            EndpointHelp(
                method="GET",
                path="/order/{order_id}",
                purpose="Check order status and payment details",
                tips=[
                    "If paid=false, you still need to send payment and submit tx_id.",
                    "Product orders show gelato_order_id and tracking_url when available.",
                ],
            ),
            EndpointHelp(
                method="POST",
                path="/feedback",
                purpose="Submit feedback on the experience",
                tips=[
                    "No auth required. Rating 1-5, message and agent fields optional.",
                    "Helps us improve the API for agent use.",
                ],
            ),
        ],
    )


# --- Menu ---


@app.get("/menu", response_model=MenuResponse, tags=["Menu"])
async def list_categories():
    """List all menu categories.

    Returns categories for cakes (demo: flavors, sizes, toppings) and
    real shippable products. Product prices are live from Gelato + CoinGecko.
    """
    cats = get_cake_categories()

    # Add products category with live item count
    product_items = await products.get_products_for_menu()
    cats.append(
        CategoryInfo(
            id="products",
            name="Shippable Products — Real items, printed & delivered",
            count=len(product_items),
            href="/menu/products",
        )
    )

    return MenuResponse(categories=cats)


@app.get("/menu/{category}", response_model=CategoryResponse, tags=["Menu"])
async def list_category_items(
    category: str,
    page: int = Query(1, ge=1, description="Page number (1-indexed)"),
):
    """List items in a menu category, paginated.

    Use the `next` field in the response to fetch additional pages.
    Item IDs from this endpoint are used when placing an order.
    For items in the 'products' category, also check GET /products/{id}/options.
    """
    if category == "products":
        all_items = await products.get_products_for_menu()
        total_pages = max(1, math.ceil(len(all_items) / ITEMS_PER_PAGE))
        page = max(1, min(page, total_pages))
        start = (page - 1) * ITEMS_PER_PAGE
        items = all_items[start : start + ITEMS_PER_PAGE]
        next_url = f"/menu/products?page={page + 1}" if page < total_pages else None
        return CategoryResponse(
            category="products", items=items, page=page, total_pages=total_pages, next=next_url
        )

    if not is_cake_category(category):
        raise HTTPException(
            status_code=404,
            detail=f"Category '{category}' not found. GET /menu to see valid categories.",
        )
    items, pg, total, next_url = get_category_items(category, page)
    return CategoryResponse(
        category=category, items=items, page=pg, total_pages=total, next=next_url
    )


# --- Products ---


@app.get("/products/{product_id}/options", response_model=ProductOptionsResponse, tags=["Products"])
async def product_options(product_id: str):
    """Get available options (sizes, colors, etc.) for a shippable product.

    Options are fetched live from Gelato's catalog.
    Use the product_id from GET /menu/products. Returns valid values for
    each option that you must include when ordering via POST /order/product.
    """
    cfg = products.get_product(product_id)
    if cfg is None:
        raise HTTPException(
            status_code=404,
            detail=f"Product '{product_id}' not found. See GET /menu/products.",
        )
    options = await products.get_product_options(product_id)
    return ProductOptionsResponse(
        product_id=product_id,
        product_name=f"{cfg['name']} — {cfg['description']}",
        options=options,
    )


# --- Orders ---


@app.post(
    "/order",
    response_model=OrderResponse,
    status_code=201,
    tags=["Orders"],
    responses={
        422: {"description": "Invalid item selection or request body"},
    },
)
async def place_cake_order(req: OrderRequest):
    """Create a cake order (demo) and get payment instructions.

    Select a flavor and size from the menu, optionally add toppings.
    The response includes the total price and a BCH address to send payment to.
    After paying, submit your transaction ID via PUT /order/{order_id}/payment.
    """
    total, invalid = calculate_total(req.flavor, req.size, req.toppings)
    if total is None:
        raise HTTPException(
            status_code=422,
            detail=f"Invalid items: {', '.join(invalid)}",
        )

    order = create_cake_order(
        flavor=req.flavor,
        size=req.size,
        toppings=req.toppings,
        message=req.message,
        total_bch=str(total),
        payment_address=SHOP_BCH_ADDRESS,
    )
    return OrderResponse(
        order_id=order["order_id"],
        status=order["status"],
        total_bch=order["total_bch"],
        payment_address=order["payment_address"],
        status_url=f"/order/{order['order_id']}",
    )


@app.post(
    "/order/product",
    response_model=OrderResponse,
    status_code=201,
    tags=["Orders"],
    responses={
        422: {"description": "Invalid product, options, or shipping address"},
        503: {"description": "Unable to resolve product or calculate price — try again shortly"},
    },
)
async def place_product_order(req: ProductOrderRequest):
    """Order a real, shippable product (t-shirt, mug, framed print).

    Select a product from GET /menu/products, choose options from
    GET /products/{id}/options, and provide a shipping address.
    The BCH price is calculated live from Gelato's cost + our margin.
    After payment, the item is printed by Gelato and shipped to your address.
    """
    cfg = products.get_product(req.product_id)
    if cfg is None:
        raise HTTPException(
            status_code=422,
            detail=f"Product '{req.product_id}' not found. See GET /menu/products.",
        )

    err = products.validate_options(req.product_id, req.options)
    if err:
        raise HTTPException(status_code=422, detail=f"Invalid options: {err}")

    # Resolve real Gelato product UID via search
    gelato_uid = await products.resolve_gelato_uid(req.product_id, req.options)
    if not gelato_uid:
        raise HTTPException(
            status_code=422,
            detail="That option combination is not available. Try different options, or check GET /products/{id}/options.",
        )

    # Live BCH price from Gelato cost + margin + exchange rate
    bch_price = await products.get_product_bch_price(req.product_id, req.options)
    if not bch_price:
        raise HTTPException(
            status_code=503,
            detail="Unable to calculate price right now. Please try again shortly.",
        )

    # Convert shipping address to Gelato format
    shipping = {
        "firstName": req.shipping_address.first_name,
        "lastName": req.shipping_address.last_name,
        "addressLine1": req.shipping_address.address_line_1,
        "addressLine2": req.shipping_address.address_line_2,
        "city": req.shipping_address.city,
        "state": req.shipping_address.state,
        "postCode": req.shipping_address.post_code,
        "country": req.shipping_address.country,
        "email": req.shipping_address.email,
        "phone": req.shipping_address.phone,
    }

    order = create_product_order(
        product_id=req.product_id,
        product_options=req.options,
        shipping_address=shipping,
        total_bch=bch_price,
        payment_address=SHOP_BCH_ADDRESS,
        design_url=cfg["design_url"],
        gelato_product_uid=gelato_uid,
    )
    return OrderResponse(
        order_id=order["order_id"],
        status=order["status"],
        total_bch=order["total_bch"],
        payment_address=order["payment_address"],
        status_url=f"/order/{order['order_id']}",
    )


@app.put(
    "/order/{order_id}/payment",
    response_model=PaymentResponse,
    tags=["Orders"],
    responses={
        402: {"description": "Payment verification failed"},
        404: {"description": "Order not found"},
        409: {"description": "Transaction already used or order already paid"},
        503: {"description": "Blockchain verification service unavailable"},
    },
)
async def pay_order(order_id: str, req: PaymentRequest):
    """Submit a BCH transaction ID to pay for an order.

    The transaction is verified against the BCH blockchain via Blockchair.
    For product orders, a real fulfillment order is placed with Gelato
    after payment is confirmed — the item will be printed and shipped.
    """
    order = get_order(order_id)
    if order is None:
        raise HTTPException(status_code=404, detail="Order not found.")

    if order["paid"]:
        raise HTTPException(status_code=409, detail="Order is already paid.")

    if is_tx_used(req.tx_id):
        raise HTTPException(
            status_code=409,
            detail="This transaction ID has already been used for another order.",
        )

    expected = Decimal(order["total_bch"])
    ok, message = await verify_transaction(req.tx_id, expected)

    if not ok:
        if "unavailable" in message.lower():
            raise HTTPException(status_code=503, detail=message)
        raise HTTPException(status_code=402, detail=message)

    updated = submit_payment(order_id, req.tx_id)

    # For product orders, place the real order with Gelato
    if updated["order_type"] == "product":
        gelato_id, gelato_msg = await place_gelato_order(
            product_uid=updated["gelato_product_uid"],
            design_url=updated["design_url"],
            shipping=updated["shipping_address"],
            our_order_id=order_id,
        )
        if gelato_id:
            update_gelato_info(order_id, gelato_id)

    return PaymentResponse(
        order_id=updated["order_id"],
        status=updated["status"],
        tx_id=req.tx_id,
        total_bch=updated["total_bch"],
    )


@app.get("/order/{order_id}", response_model=OrderStatusResponse, tags=["Orders"])
async def check_order(order_id: str):
    """Check the current status of an order.

    If status is 'awaiting_payment', send BCH to the payment_address
    and then PUT /order/{order_id}/payment with your transaction ID.
    Product orders show gelato_order_id and tracking_url when available.
    """
    order = get_order(order_id)
    if order is None:
        raise HTTPException(status_code=404, detail="Order not found.")

    return OrderStatusResponse(
        order_id=order["order_id"],
        status=order["status"],
        order_type=order["order_type"],
        total_bch=order["total_bch"],
        payment_address=order["payment_address"],
        paid=order["paid"],
        tx_id=order["tx_id"],
        # Cake fields
        flavor=order.get("flavor"),
        size=order.get("size"),
        toppings=order.get("toppings"),
        message=order.get("message"),
        # Product fields
        product_id=order.get("product_id"),
        product_options=order.get("product_options"),
        gelato_order_id=order.get("gelato_order_id"),
        tracking_url=order.get("tracking_url"),
    )


# --- Feedback ---


@app.post(
    "/feedback",
    response_model=FeedbackResponse,
    status_code=201,
    tags=["Feedback"],
)
async def submit_feedback(req: FeedbackRequest):
    """Submit feedback on the ordering experience.

    No authentication required. This helps us learn whether agents
    find the interface easy to discover and use.
    """
    entry = create_feedback(rating=req.rating, message=req.message, agent=req.agent)
    return FeedbackResponse(feedback_id=entry["feedback_id"])
