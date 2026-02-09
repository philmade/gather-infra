"""Menu data and browsing logic for cakes (demo).

Categories and items are hardcoded for the demo.
Products (real shippable goods) are handled separately in products.py.
"""

import math
from models import CategoryInfo, MenuItem

ITEMS_PER_PAGE = 5

# --- Raw menu data (cakes only) ---

_FLAVORS: list[MenuItem] = [
    MenuItem(id="chocolate", name="Chocolate Fudge", available=True, base_price_bch="0.010"),
    MenuItem(id="vanilla", name="Classic Vanilla", available=True, base_price_bch="0.009"),
    MenuItem(id="red_velvet", name="Red Velvet", available=True, base_price_bch="0.011"),
    MenuItem(id="lemon", name="Lemon Zest", available=True, base_price_bch="0.010"),
    MenuItem(id="carrot", name="Carrot Cake", available=True, base_price_bch="0.010"),
]

_SIZES: list[MenuItem] = [
    MenuItem(id="small", name="Small (6 inch)", available=True, base_price_bch="0.000"),
    MenuItem(id="medium", name="Medium (8 inch)", available=True, base_price_bch="0.005"),
    MenuItem(id="large", name="Large (10 inch)", available=True, base_price_bch="0.010"),
]

_TOPPINGS: list[MenuItem] = [
    MenuItem(id="sprinkles", name="Rainbow Sprinkles", available=True, base_price_bch="0.001"),
    MenuItem(id="caramel_drizzle", name="Caramel Drizzle", available=True, base_price_bch="0.002"),
    MenuItem(id="fresh_berries", name="Fresh Berries", available=True, base_price_bch="0.003"),
    MenuItem(id="chocolate_shavings", name="Chocolate Shavings", available=True, base_price_bch="0.002"),
    MenuItem(id="whipped_cream", name="Whipped Cream", available=True, base_price_bch="0.001"),
    MenuItem(id="fondant_flowers", name="Fondant Flowers", available=True, base_price_bch="0.004"),
    MenuItem(id="edible_gold", name="Edible Gold Leaf", available=True, base_price_bch="0.008"),
    MenuItem(id="custom_text", name="Custom Text Topper", available=True, base_price_bch="0.002"),
]

CATEGORIES: dict[str, list[MenuItem]] = {
    "flavors": _FLAVORS,
    "sizes": _SIZES,
    "toppings": _TOPPINGS,
}

CATEGORY_NAMES: dict[str, str] = {
    "flavors": "Cake Flavors",
    "sizes": "Cake Sizes",
    "toppings": "Toppings & Add-ons",
}


def get_cake_categories() -> list[CategoryInfo]:
    """Return cake category listing with hrefs."""
    return [
        CategoryInfo(
            id=cat_id,
            name=CATEGORY_NAMES[cat_id],
            count=len(items),
            href=f"/menu/{cat_id}",
        )
        for cat_id, items in CATEGORIES.items()
    ]


def get_category_items(
    category: str, page: int = 1
) -> tuple[list[MenuItem], int, int, str | None]:
    """Return paginated items for a cake category.

    Returns (items, page, total_pages, next_url_or_none).
    Raises KeyError if category doesn't exist.
    """
    all_items = CATEGORIES[category]
    total_pages = max(1, math.ceil(len(all_items) / ITEMS_PER_PAGE))
    page = max(1, min(page, total_pages))

    start = (page - 1) * ITEMS_PER_PAGE
    end = start + ITEMS_PER_PAGE
    items = all_items[start:end]

    next_url = f"/menu/{category}?page={page + 1}" if page < total_pages else None
    return items, page, total_pages, next_url


def is_cake_category(category: str) -> bool:
    """Check if a category is a valid cake category."""
    return category in CATEGORIES


def get_item_price(category: str, item_id: str) -> str | None:
    """Look up the BCH price for a specific cake item. Returns None if not found."""
    for item in CATEGORIES.get(category, []):
        if item.id == item_id:
            return item.base_price_bch
    return None
