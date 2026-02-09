"""Dynamic Gelato product catalog proxy.

Thin config tells us WHICH products to sell and HOW to present them.
Everything else — available options (sizes, colors), product UIDs,
and USD pricing — comes live from Gelato's catalog API.

BCH prices are computed from Gelato's USD cost + our margin,
converted at the live exchange rate via CoinGecko.
"""

import asyncio
import os
import time

import httpx
from dotenv import load_dotenv

from models import MenuItem

load_dotenv()

GELATO_API_KEY = os.environ.get("GELATO_API_KEY", "")
GELATO_CATALOG_URL = "https://product.gelatoapis.com/v3"
COINGECKO_URL = "https://api.coingecko.com/api/v3/simple/price"

# ---------------------------------------------------------------------------
# Product configuration — the ONLY manual part.
#
# fixed_filters: always applied when searching Gelato (defines "our" product).
# agent_options: which Gelato attributes the agent picks from.
#                Keys are friendly names; values are Gelato attribute UIDs.
# reference_variant: default option values used for menu "from" pricing.
# margin_pct: markup over Gelato's cost price.
# design_url: artwork sent to Gelato for printing.
# ---------------------------------------------------------------------------

CATALOG_CONFIG: dict[str, dict] = {
    "t-shirt": {
        "gelato_catalog": "t-shirts",
        "name": "T-Shirt",
        "description": "Unisex crewneck, printed front",
        "fixed_filters": {
            "GarmentCut": "unisex",
            "GarmentSubcategory": "crewneck",
            "GarmentQuality": "classic",
            "GarmentPrint": "4-0",
            "ProductStatus": "activated",
        },
        "agent_options": {
            "size": "GarmentSize",
            "color": "GarmentColor",
        },
        "reference_variant": {"size": "M", "color": "white"},
        "design_url": "https://placehold.co/4000x5000/png?text=Design+Placeholder",
        "margin_pct": 40,
    },
    "mug": {
        "gelato_catalog": "mugs",
        "name": "Ceramic Mug",
        "description": "White ceramic, printed wrap",
        "fixed_filters": {
            "MugMaterial": "ceramic-white",
            "ProductStatus": "activated",
        },
        "agent_options": {
            "size": "MugSize",
        },
        "reference_variant": {"size": "11-oz"},
        "design_url": "https://placehold.co/4000x2000/png?text=Design+Placeholder",
        "margin_pct": 40,
    },
    "framed-print": {
        "gelato_catalog": "framed-posters",
        "name": "Framed Print",
        "description": "Black wood frame, plexiglass front",
        "fixed_filters": {
            "FrameColor": "black",
            "FrameMaterial": "wood",
            "ProductStatus": "activated",
        },
        "agent_options": {
            "size": "PaperFormat",
            "orientation": "Orientation",
        },
        "reference_variant": {"size": "a3", "orientation": "ver"},
        "design_url": "https://placehold.co/3000x4000/png?text=Design+Placeholder",
        "margin_pct": 40,
    },
}

# ---------------------------------------------------------------------------
# Cache — simple TTL dict.  Stale data returned if a refresh fails.
# ---------------------------------------------------------------------------

_cache: dict[str, dict] = {}

CATALOG_TTL = 3600  # 1 hour — attribute lists rarely change
PRICE_TTL = 1800    # 30 min — Gelato cost prices
RATE_TTL = 300      # 5 min — BCH exchange rate


async def _get_cached(key: str, fetch_fn, ttl: int):
    entry = _cache.get(key)
    if entry and (time.time() - entry["t"]) < ttl:
        return entry["d"]
    try:
        data = await fetch_fn()
        _cache[key] = {"d": data, "t": time.time()}
        return data
    except Exception:
        if entry:  # stale but usable
            return entry["d"]
        raise


def _gelato_headers() -> dict:
    return {"X-API-KEY": GELATO_API_KEY}


# Apparel sizes in standard display order
_APPAREL_SIZE_ORDER = {s: i for i, s in enumerate(
    ["XS", "S", "M", "L", "XL", "2XL", "3XL", "4XL", "5XL"]
)}


# ---------------------------------------------------------------------------
# Raw Gelato API calls
# ---------------------------------------------------------------------------

async def _fetch_valid_options(product_id: str) -> dict[str, list[str]]:
    """Search Gelato with our fixed_filters, paginate, extract unique values.

    Only returns option values that actually exist for our product definition.
    This replaces the old catalog-attributes approach which returned ALL values
    across ALL variants in a catalog (most of which didn't work with our filters).
    """
    cfg = CATALOG_CONFIG[product_id]
    attr_filters = {k: [v] for k, v in cfg["fixed_filters"].items()}

    all_products = []
    offset = 0
    async with httpx.AsyncClient(timeout=15.0) as client:
        while True:
            r = await client.post(
                f"{GELATO_CATALOG_URL}/catalogs/{cfg['gelato_catalog']}/products:search",
                headers=_gelato_headers(),
                json={"attributeFilters": attr_filters, "limit": 100, "offset": offset},
            )
            r.raise_for_status()
            batch = r.json().get("products", [])
            if not batch:
                break
            all_products.extend(batch)
            offset += len(batch)

    options: dict[str, list[str]] = {}
    for opt_name, gelato_attr in cfg["agent_options"].items():
        values = sorted({p["attributes"].get(gelato_attr, "") for p in all_products} - {""})
        # Sort apparel sizes in standard order (XS → 5XL) instead of alphabetical
        if gelato_attr == "GarmentSize":
            values.sort(key=lambda v: _APPAREL_SIZE_ORDER.get(v, 99))
        options[opt_name] = values

    return options


async def _search_gelato_product(catalog_uid: str, filters: dict[str, str]) -> str | None:
    """Search Gelato catalog → first matching product UID, or None."""
    attr_filters = {k: [v] for k, v in filters.items()}
    async with httpx.AsyncClient(timeout=10.0) as client:
        r = await client.post(
            f"{GELATO_CATALOG_URL}/catalogs/{catalog_uid}/products:search",
            headers=_gelato_headers(),
            json={"attributeFilters": attr_filters, "limit": 1},
        )
        r.raise_for_status()
    products = r.json().get("products", [])
    return products[0]["productUid"] if products else None


async def _fetch_product_price_usd(product_uid: str) -> float | None:
    """GET /v3/products/{uid}/prices → USD cost price."""
    async with httpx.AsyncClient(timeout=10.0) as client:
        r = await client.get(
            f"{GELATO_CATALOG_URL}/products/{product_uid}/prices",
            headers=_gelato_headers(),
        )
        r.raise_for_status()
    prices = r.json()
    if isinstance(prices, list) and prices:
        return float(prices[0].get("price", 0))
    return None


async def _fetch_bch_rate() -> float:
    """CoinGecko → BCH price in USD."""
    async with httpx.AsyncClient(timeout=10.0) as client:
        r = await client.get(
            COINGECKO_URL,
            params={"ids": "bitcoin-cash", "vs_currencies": "usd"},
        )
        r.raise_for_status()
    return float(r.json()["bitcoin-cash"]["usd"])


# ---------------------------------------------------------------------------
# Cached wrappers
# ---------------------------------------------------------------------------

async def _get_valid_options(product_id: str) -> dict[str, list[str]]:
    return await _get_cached(
        f"valid_options:{product_id}",
        lambda: _fetch_valid_options(product_id),
        CATALOG_TTL,
    )


async def get_bch_rate() -> float:
    return await _get_cached("bch_rate", _fetch_bch_rate, RATE_TTL)


async def _get_gelato_uid(product_id: str, agent_choices: dict[str, str]) -> str | None:
    """Resolve agent's option choices to a Gelato product UID via search."""
    cfg = CATALOG_CONFIG.get(product_id)
    if not cfg:
        return None
    filters = dict(cfg["fixed_filters"])
    for opt_name, opt_value in agent_choices.items():
        gelato_attr = cfg["agent_options"].get(opt_name)
        if gelato_attr:
            filters[gelato_attr] = opt_value

    cache_key = f"uid:{product_id}:{sorted(agent_choices.items())}"
    return await _get_cached(
        cache_key,
        lambda: _search_gelato_product(cfg["gelato_catalog"], filters),
        CATALOG_TTL,
    )


async def _get_product_price_usd(product_uid: str) -> float | None:
    return await _get_cached(
        f"price_usd:{product_uid}",
        lambda: _fetch_product_price_usd(product_uid),
        PRICE_TTL,
    )


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

def get_product(product_id: str) -> dict | None:
    """Get product config (sync — just reads the dict)."""
    return CATALOG_CONFIG.get(product_id)


async def get_product_options(product_id: str) -> dict[str, list[str]] | None:
    """Available options for a product — only values that actually work.

    Determined by searching Gelato with our fixed_filters and extracting
    unique attribute values from the results.  Every returned value is
    guaranteed to resolve to a real product UID.
    """
    if product_id not in CATALOG_CONFIG:
        return None
    return await _get_valid_options(product_id)


def validate_options(product_id: str, options: dict[str, str]) -> str | None:
    """Basic sync check: all required option keys present, no extras.

    Returns error message or None.  Value validation is done by Gelato search
    (returns 0 results for invalid combos).
    """
    cfg = CATALOG_CONFIG.get(product_id)
    if not cfg:
        return f"Unknown product: {product_id}"
    for opt_name in cfg["agent_options"]:
        if opt_name not in options:
            return f"Missing required option '{opt_name}'"
    for opt_name in options:
        if opt_name not in cfg["agent_options"]:
            return f"Unknown option '{opt_name}'. Valid options: {list(cfg['agent_options'].keys())}"
    return None


async def resolve_gelato_uid(product_id: str, agent_choices: dict[str, str]) -> str | None:
    """Resolve agent's choices to a real Gelato product UID."""
    return await _get_gelato_uid(product_id, agent_choices)


async def get_product_bch_price(
    product_id: str,
    agent_choices: dict[str, str] | None = None,
) -> str | None:
    """BCH price = Gelato USD cost * (1 + margin) / BCH rate.

    If agent_choices is None, uses reference_variant (for menu "from" pricing).
    """
    cfg = CATALOG_CONFIG.get(product_id)
    if not cfg:
        return None
    choices = agent_choices or cfg["reference_variant"]
    uid = await _get_gelato_uid(product_id, choices)
    if not uid:
        return None
    usd_cost = await _get_product_price_usd(uid)
    if usd_cost is None:
        return None
    usd_with_margin = usd_cost * (1 + cfg["margin_pct"] / 100)
    bch_rate = await get_bch_rate()
    bch = usd_with_margin / bch_rate
    return f"{bch:.6f}"


async def get_products_for_menu() -> list[MenuItem]:
    """MenuItem list for all configured products, with live BCH prices."""
    if not GELATO_API_KEY:
        return [
            MenuItem(
                id=pid,
                name=f"{cfg['name']} — {cfg['description']}",
                available=False,
                base_price_bch="0.000000",
            )
            for pid, cfg in CATALOG_CONFIG.items()
        ]

    async def _build(pid: str, cfg: dict) -> MenuItem:
        try:
            price = await get_product_bch_price(pid)
        except Exception:
            price = None
        return MenuItem(
            id=pid,
            name=f"{cfg['name']} — {cfg['description']}",
            available=price is not None,
            base_price_bch=price or "0.000000",
        )

    return list(await asyncio.gather(
        *[_build(pid, cfg) for pid, cfg in CATALOG_CONFIG.items()]
    ))
