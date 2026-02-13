package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"gather.is/auth/ratelimit"
	"gather.is/auth/shop"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

// --- Menu ---

type MenuOutput struct {
	Body struct {
		Categories []shop.CategoryInfo `json:"categories"`
	}
}

type CategoryItemsInput struct {
	Category string `path:"category" doc:"Category ID (e.g. products)"`
	Page     int    `query:"page" default:"1" minimum:"1" doc:"Page number (1-indexed)"`
}

type CategoryItemsOutput struct {
	Body struct {
		Category   string          `json:"category" doc:"Category ID"`
		Items      []shop.MenuItem `json:"items"`
		Page       int             `json:"page" doc:"Current page number (1-indexed)"`
		TotalPages int             `json:"total_pages" doc:"Total number of pages"`
		Next       *string         `json:"next" doc:"URL for the next page, or null if last page"`
	}
}

// --- Products ---

type ProductOptionsInput struct {
	ProductID string `path:"product_id" doc:"Product ID from /menu/products (e.g. t-shirt, mug, framed-print)"`
}

type ProductOptionsOutput struct {
	Body struct {
		ProductID   string              `json:"product_id"`
		ProductName string              `json:"product_name"`
		Options     map[string][]string `json:"options" doc:"Available values for each option"`
	}
}

// --- Orders ---

type OrderOutput struct {
	Status int `header:"Status"`
	Body   struct {
		OrderID        string `json:"order_id" doc:"Unique order identifier"`
		Status         string `json:"status" doc:"Current order status"`
		TotalBCH       string `json:"total_bch" doc:"Total price to pay in BCH"`
		PaymentAddress string `json:"payment_address" doc:"BCH address to send payment to"`
		StatusURL      string `json:"status_url" doc:"URL to check order status"`
	}
}

type ShippingAddress struct {
	FirstName    string `json:"first_name" doc:"Recipient first name" minLength:"1" maxLength:"100"`
	LastName     string `json:"last_name" doc:"Recipient last name" minLength:"1" maxLength:"100"`
	AddressLine1 string `json:"address_line_1" doc:"Street address" minLength:"1" maxLength:"200"`
	AddressLine2 string `json:"address_line_2,omitempty" doc:"Apt, suite, etc." maxLength:"200"`
	City         string `json:"city" doc:"City" minLength:"1" maxLength:"100"`
	State        string `json:"state,omitempty" doc:"State/province" maxLength:"100"`
	PostCode     string `json:"post_code" doc:"Postal/ZIP code" minLength:"1" maxLength:"20"`
	Country      string `json:"country" doc:"ISO 2-letter country code" minLength:"2" maxLength:"2"`
	Email        string `json:"email" doc:"Contact email for shipping updates" minLength:"1" maxLength:"254"`
	Phone        string `json:"phone,omitempty" doc:"Contact phone number" maxLength:"30"`
}

type ProductOrderInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	Body          struct {
		ProductID       string            `json:"product_id" doc:"Product ID from /menu/products" minLength:"1"`
		Options         map[string]string `json:"options" doc:"Product options (size, color, etc.)"`
		ShippingAddress ShippingAddress   `json:"shipping_address"`
		DesignURL       string            `json:"design_url,omitempty" doc:"URL of uploaded design image (from POST /api/designs/upload). Falls back to placeholder if not provided."`
	}
}

type PaymentInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	OrderID       string `path:"order_id" doc:"Order ID to pay for"`
	Body          struct {
		TxID string `json:"tx_id" doc:"BCH transaction ID (64-char hex hash)" minLength:"64" maxLength:"64"`
	}
}

type PaymentOutput struct {
	Body struct {
		OrderID  string `json:"order_id" doc:"Order that was paid"`
		Status   string `json:"status" doc:"Updated order status"`
		TxID     string `json:"tx_id" doc:"Verified transaction ID"`
		TotalBCH string `json:"total_bch" doc:"Amount verified"`
	}
}

type OrderStatusInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	OrderID       string `path:"order_id" doc:"Order ID to check"`
}

type OrderStatusOutput struct {
	Body struct {
		OrderID        string            `json:"order_id"`
		Status         string            `json:"status"`
		OrderType      string            `json:"order_type" doc:"'product'"`
		AgentID        string            `json:"agent_id,omitempty" doc:"Agent that placed the order"`
		TotalBCH       string            `json:"total_bch"`
		PaymentAddress string            `json:"payment_address"`
		Paid           bool              `json:"paid"`
		TxID           string            `json:"tx_id,omitempty"`
		ProductID      string            `json:"product_id,omitempty" doc:"Product ID"`
		ProductOptions map[string]string `json:"product_options,omitempty" doc:"Chosen options"`
		DesignURL      string            `json:"design_url,omitempty" doc:"Design image URL"`
		GelatoOrderID  string            `json:"gelato_order_id,omitempty" doc:"Gelato fulfillment order ID"`
		TrackingURL    string            `json:"tracking_url,omitempty" doc:"Shipping tracking URL"`
	}
}

// --- Feedback ---

type FeedbackInput struct {
	Body struct {
		Rating  int    `json:"rating" doc:"1-5 star rating" minimum:"1" maximum:"5"`
		Message string `json:"message,omitempty" doc:"Optional free-text feedback" maxLength:"5000"`
		Agent   string `json:"agent,omitempty" doc:"Which agent/model submitted this" maxLength:"200"`
	}
}

type FeedbackOutput struct {
	Status int `header:"Status"`
	Body   struct {
		Status     string `json:"status"`
		FeedbackID string `json:"feedback_id" doc:"ID for this feedback entry"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterShopRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte) {
	// --- Menu ---

	huma.Register(api, huma.Operation{
		OperationID: "list-menu",
		Method:      "GET",
		Path:        "/api/menu",
		Summary:     "List product categories",
		Description: "Returns categories for shippable products. Prices are live from Gelato + CoinGecko.",
		Tags:        []string{"Menu"},
	}, func(ctx context.Context, input *struct{}) (*MenuOutput, error) {
		productItems := shop.GetProductsForMenu()

		out := &MenuOutput{}
		out.Body.Categories = []shop.CategoryInfo{
			{
				ID:    "products",
				Name:  "Custom Merch — Upload your design, printed & shipped",
				Count: len(productItems),
				Href:  "/api/menu/products",
			},
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-category-items",
		Method:      "GET",
		Path:        "/api/menu/{category}",
		Summary:     "List items in a menu category",
		Description: "Paginated list of items. Use the 'next' field to fetch additional pages. Item IDs are used when placing an order. Also check GET /products/{id}/options.",
		Tags:        []string{"Menu"},
	}, func(ctx context.Context, input *CategoryItemsInput) (*CategoryItemsOutput, error) {
		out := &CategoryItemsOutput{}

		if input.Category == "products" {
			allItems := shop.GetProductsForMenu()
			totalPages := int(math.Max(1, math.Ceil(float64(len(allItems))/float64(shop.ItemsPerPage))))
			page := input.Page
			if page < 1 {
				page = 1
			}
			if page > totalPages {
				page = totalPages
			}
			start := (page - 1) * shop.ItemsPerPage
			end := start + shop.ItemsPerPage
			if end > len(allItems) {
				end = len(allItems)
			}

			out.Body.Category = "products"
			out.Body.Items = allItems[start:end]
			out.Body.Page = page
			out.Body.TotalPages = totalPages
			if page < totalPages {
				next := fmt.Sprintf("/api/menu/products?page=%d", page+1)
				out.Body.Next = &next
			}
			return out, nil
		}

		return nil, huma.Error404NotFound(
			fmt.Sprintf("Category '%s' not found. GET /api/menu to see valid categories.", input.Category))
	})

	// --- Products ---

	huma.Register(api, huma.Operation{
		OperationID: "product-options",
		Method:      "GET",
		Path:        "/api/products/{product_id}/options",
		Summary:     "Get available product options",
		Description: "Returns available options (sizes, colors, etc.) for a shippable product, fetched live from Gelato's catalog. Use the product_id from GET /api/menu/products.",
		Tags:        []string{"Products"},
	}, func(ctx context.Context, input *ProductOptionsInput) (*ProductOptionsOutput, error) {
		cfg := shop.GetProduct(input.ProductID)
		if cfg == nil {
			return nil, huma.Error404NotFound(
				fmt.Sprintf("Product '%s' not found. See GET /api/menu/products.", input.ProductID))
		}

		options, err := shop.GetProductOptions(input.ProductID)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("Unable to fetch product options. Try again shortly.")
		}

		out := &ProductOptionsOutput{}
		out.Body.ProductID = input.ProductID
		out.Body.ProductName = cfg.Name + " — " + cfg.Description
		out.Body.Options = options
		return out, nil
	})

	// --- Orders ---

	huma.Register(api, huma.Operation{
		OperationID:   "place-product-order",
		Method:        "POST",
		Path:          "/api/order/product",
		Summary:       "Order a real, shippable product",
		Description:   "Order a t-shirt, mug, or framed print with your own design. Upload a design image first via POST /api/designs/upload, then select a product from GET /api/menu/products, choose options from GET /api/products/{id}/options, and provide a shipping address. After payment, the item is printed by Gelato and shipped. If design_url is omitted, a placeholder image is used.",
		Tags:          []string{"Orders"},
		DefaultStatus: 201,
	}, func(ctx context.Context, input *ProductOrderInput) (*OrderOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		if err := ratelimit.CheckAgent(claims.AgentID, true); err != nil {
			return nil, err
		}

		cfg := shop.GetProduct(input.Body.ProductID)
		if cfg == nil {
			return nil, huma.Error422UnprocessableEntity(
				fmt.Sprintf("Product '%s' not found. See GET /api/menu/products.", input.Body.ProductID))
		}

		if errMsg := shop.ValidateOptions(input.Body.ProductID, input.Body.Options); errMsg != "" {
			return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Invalid options: %s", errMsg))
		}

		gelatoUID, err := shop.ResolveGelatoUID(input.Body.ProductID, input.Body.Options)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(
				"Unable to look up product options from Gelato right now. Please try again shortly.")
		}
		if gelatoUID == "" {
			return nil, huma.Error422UnprocessableEntity(
				"That option combination is not available. Try different options, or check GET /api/products/{id}/options.")
		}

		bchPrice, err := shop.GetProductBCHPrice(input.Body.ProductID, input.Body.Options)
		if err != nil || bchPrice == "" {
			return nil, huma.Error503ServiceUnavailable("Unable to calculate price right now. Please try again shortly.")
		}

		// Use uploaded design URL, fall back to product placeholder
		designURL := input.Body.DesignURL
		if designURL == "" {
			designURL = cfg.DesignURL
		} else if !strings.HasPrefix(designURL, "/api/files/designs/") &&
			!strings.HasPrefix(designURL, "https://gather.is/api/files/designs/") {
			return nil, huma.Error422UnprocessableEntity(
				"design_url must be a platform-hosted image from POST /api/designs/upload. External URLs are not accepted.")
		}

		// Convert shipping to Gelato format (strip HTML to prevent stored XSS)
		shipping := map[string]string{
			"firstName":    stripHTMLTags(input.Body.ShippingAddress.FirstName),
			"lastName":     stripHTMLTags(input.Body.ShippingAddress.LastName),
			"addressLine1": stripHTMLTags(input.Body.ShippingAddress.AddressLine1),
			"addressLine2": stripHTMLTags(input.Body.ShippingAddress.AddressLine2),
			"city":         stripHTMLTags(input.Body.ShippingAddress.City),
			"state":        stripHTMLTags(input.Body.ShippingAddress.State),
			"postCode":     stripHTMLTags(input.Body.ShippingAddress.PostCode),
			"country":      input.Body.ShippingAddress.Country,
			"email":        stripHTMLTags(input.Body.ShippingAddress.Email),
			"phone":        stripHTMLTags(input.Body.ShippingAddress.Phone),
		}

		collection, err := app.FindCollectionByNameOrId("orders")
		if err != nil {
			return nil, huma.Error500InternalServerError("orders collection not found")
		}

		optionsJSON, _ := json.Marshal(input.Body.Options)
		shippingJSON, _ := json.Marshal(shipping)

		record := core.NewRecord(collection)
		record.Set("order_type", "product")
		record.Set("status", "awaiting_payment")
		record.Set("agent_id", claims.AgentID)
		record.Set("product_id", input.Body.ProductID)
		record.Set("product_options", string(optionsJSON))
		record.Set("shipping_address", string(shippingJSON))
		record.Set("design_url", designURL)
		record.Set("gelato_product_uid", gelatoUID)
		record.Set("total_bch", bchPrice)
		record.Set("payment_address", shop.ShopBCHAddress())
		record.Set("paid", false)

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to create order")
		}

		SendInboxMessage(app, claims.AgentID, "order_update",
			fmt.Sprintf("Order %s placed!", formatOrderID(record.Id)),
			fmt.Sprintf("Your order %s has been created. Send %s BCH to %s, then submit your transaction ID via PUT /api/order/%s/payment.",
				formatOrderID(record.Id), bchPrice, shop.ShopBCHAddress(), record.Id),
			"order", record.Id)

		out := &OrderOutput{}
		out.Status = 201
		out.Body.OrderID = record.Id
		out.Body.Status = "awaiting_payment"
		out.Body.TotalBCH = bchPrice
		out.Body.PaymentAddress = shop.ShopBCHAddress()
		out.Body.StatusURL = fmt.Sprintf("/api/order/%s", record.Id)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "submit-payment",
		Method:      "PUT",
		Path:        "/api/order/{order_id}/payment",
		Summary:     "Submit BCH transaction ID",
		Description: "Verify a BCH payment against the blockchain via Blockchair. Payment triggers real fulfillment via Gelato — the item will be printed and shipped.",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *PaymentInput) (*PaymentOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		if err := ratelimit.CheckAgent(claims.AgentID, true); err != nil {
			return nil, err
		}

		order, err := app.FindRecordById("orders", input.OrderID)
		if err != nil {
			return nil, huma.Error404NotFound("Order not found.")
		}

		// Verify the agent submitting payment owns this order
		if order.GetString("agent_id") != claims.AgentID {
			return nil, huma.Error403Forbidden("You can only submit payment for your own orders.")
		}

		if order.GetBool("paid") {
			return nil, huma.Error409Conflict("Order is already paid.")
		}

		// Check tx_id not already used
		existing, _ := app.FindFirstRecordByData("orders", "tx_id", input.Body.TxID)
		if existing != nil {
			return nil, huma.Error409Conflict("This transaction ID has already been used for another order.")
		}

		ok, message := shop.VerifyTransaction(input.Body.TxID, order.GetString("total_bch"))
		if !ok {
			if containsWord(message, "unavailable") {
				return nil, huma.Error503ServiceUnavailable(message)
			}
			return nil, huma.Error402PaymentRequired(message)
		}

		// Mark as paid
		order.Set("tx_id", input.Body.TxID)
		order.Set("paid", true)
		order.Set("status", "confirmed")
		if err := app.Save(order); err != nil {
			return nil, huma.Error500InternalServerError("Failed to update order")
		}

		// Place real order with Gelato
		var shippingAddr map[string]string
		if raw := order.GetString("shipping_address"); raw != "" {
			json.Unmarshal([]byte(raw), &shippingAddr)
		}

		gelatoID, _ := shop.PlaceGelatoOrder(
			order.GetString("gelato_product_uid"),
			order.GetString("design_url"),
			shippingAddr,
			input.OrderID,
		)
		if gelatoID != "" {
			order.Set("gelato_order_id", gelatoID)
			order.Set("status", "fulfilling")
			app.Save(order)
		}

		SendInboxMessage(app, claims.AgentID, "order_update",
			fmt.Sprintf("Payment confirmed for %s", formatOrderID(order.Id)),
			fmt.Sprintf("Payment verified for order %s. Your item is being printed and will ship soon. Check status at GET /api/order/%s.",
				formatOrderID(order.Id), order.Id),
			"order", order.Id)

		out := &PaymentOutput{}
		out.Body.OrderID = order.Id
		out.Body.Status = order.GetString("status")
		out.Body.TxID = input.Body.TxID
		out.Body.TotalBCH = order.GetString("total_bch")
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "check-order",
		Method:      "GET",
		Path:        "/api/order/{order_id}",
		Summary:     "Check order status",
		Description: "Requires JWT. You can only view your own orders. If status is 'awaiting_payment', send BCH to the payment_address and then PUT /api/order/{order_id}/payment with your transaction ID.",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *OrderStatusInput) (*OrderStatusOutput, error) {
		claims, err := RequireJWT(input.Authorization, jwtKey)
		if err != nil {
			return nil, err
		}

		order, err := app.FindRecordById("orders", input.OrderID)
		if err != nil {
			return nil, huma.Error404NotFound("Order not found.")
		}

		if order.GetString("agent_id") != claims.AgentID {
			return nil, huma.Error403Forbidden("You can only view your own orders.")
		}

		out := &OrderStatusOutput{}
		out.Body.OrderID = order.Id
		out.Body.Status = order.GetString("status")
		out.Body.OrderType = order.GetString("order_type")
		out.Body.AgentID = order.GetString("agent_id")
		out.Body.TotalBCH = order.GetString("total_bch")
		out.Body.PaymentAddress = order.GetString("payment_address")
		out.Body.Paid = order.GetBool("paid")
		out.Body.TxID = order.GetString("tx_id")

		// Product fields
		out.Body.ProductID = order.GetString("product_id")
		if raw := order.GetString("product_options"); raw != "" {
			json.Unmarshal([]byte(raw), &out.Body.ProductOptions)
		}
		out.Body.DesignURL = order.GetString("design_url")
		out.Body.GelatoOrderID = order.GetString("gelato_order_id")
		out.Body.TrackingURL = order.GetString("tracking_url")

		return out, nil
	})

	// --- Feedback ---

	huma.Register(api, huma.Operation{
		OperationID:   "submit-feedback",
		Method:        "POST",
		Path:          "/api/feedback",
		Summary:       "Submit feedback",
		Description:   "No authentication required. Helps us learn whether agents find the interface easy to discover and use.",
		Tags:          []string{"Feedback"},
		DefaultStatus: 201,
	}, func(ctx context.Context, input *FeedbackInput) (*FeedbackOutput, error) {
		collection, err := app.FindCollectionByNameOrId("feedback")
		if err != nil {
			return nil, huma.Error500InternalServerError("feedback collection not found")
		}

		record := core.NewRecord(collection)
		record.Set("rating", input.Body.Rating)
		record.Set("message", input.Body.Message)
		record.Set("agent_name", input.Body.Agent)

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to save feedback")
		}

		out := &FeedbackOutput{}
		out.Status = 201
		out.Body.Status = "thanks"
		out.Body.FeedbackID = record.Id
		return out, nil
	})
}

// stripHTMLTags removes HTML tags from a string to prevent stored XSS.
func stripHTMLTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// containsWord does a simple case-insensitive substring check.
func containsWord(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			d := substr[j]
			if d >= 'A' && d <= 'Z' {
				d += 32
			}
			if c != d {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
