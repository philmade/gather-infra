package shop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const gelatoOrdersURL = "https://order.gelatoapis.com/v4/orders"

func gelatoAPIKey() string {
	return os.Getenv("GELATO_API_KEY")
}

// PlaceGelatoOrder places a real order with Gelato for print-on-demand fulfillment.
// Returns (gelatoOrderID, message). gelatoOrderID is empty on failure.
func PlaceGelatoOrder(productUID, designURL string, shipping map[string]string, ourOrderID string) (string, string) {
	apiKey := gelatoAPIKey()
	if apiKey == "" {
		return "", "Gelato API key not configured. Set GELATO_API_KEY to enable real order fulfillment."
	}

	payload := map[string]interface{}{
		"orderType":           "order",
		"orderReferenceId":    ourOrderID,
		"customerReferenceId": ourOrderID,
		"currency":            "USD",
		"items": []map[string]interface{}{
			{
				"itemReferenceId": ourOrderID + "-item-1",
				"productUid":      productUID,
				"files": []map[string]string{
					{"type": "default", "url": designURL},
				},
				"quantity": 1,
			},
		},
		"shippingAddress": shipping,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "Failed to build Gelato order payload."
	}

	req, err := http.NewRequest("POST", gelatoOrdersURL, bytes.NewReader(body))
	if err != nil {
		return "", "Failed to create Gelato request."
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("User-Agent", "Gather/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "Gelato API unavailable. Order saved — fulfillment will be retried."
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		var data map[string]interface{}
		if err := json.Unmarshal(respBody, &data); err == nil {
			if id, ok := data["id"]; ok {
				return fmt.Sprintf("%v", id), "Order placed with Gelato for fulfillment."
			}
			if id, ok := data["orderId"]; ok {
				return fmt.Sprintf("%v", id), "Order placed with Gelato for fulfillment."
			}
		}
		return "unknown", "Order placed with Gelato for fulfillment."
	}

	snippet := string(respBody)
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}
	return "", fmt.Sprintf("Gelato API error (%d): %s", resp.StatusCode, snippet)
}
