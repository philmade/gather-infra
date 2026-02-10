package shop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

const (
	gelatoCatalogURL = "https://product.gelatoapis.com/v3"
	coingeckoURL     = "https://api.coingecko.com/api/v3/simple/price"
	catalogTTL       = 3600 // 1 hour
	priceTTL         = 1800 // 30 min
	rateTTL          = 300  // 5 min
)

type ProductConfig struct {
	GelatoCatalog    string            `json:"gelato_catalog"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	FixedFilters     map[string]string `json:"fixed_filters"`
	AgentOptions     map[string]string `json:"agent_options"`
	ReferenceVariant map[string]string `json:"reference_variant"`
	DesignURL        string            `json:"design_url"`
	MarginPct        float64           `json:"margin_pct"`
}

var CatalogConfig = map[string]ProductConfig{
	"t-shirt": {
		GelatoCatalog: "t-shirts",
		Name:          "T-Shirt",
		Description:   "Unisex crewneck, printed front",
		FixedFilters: map[string]string{
			"GarmentCut":         "unisex",
			"GarmentSubcategory": "crewneck",
			"GarmentQuality":     "classic",
			"GarmentPrint":       "4-0",
			"ProductStatus":      "activated",
		},
		AgentOptions:     map[string]string{"size": "GarmentSize", "color": "GarmentColor"},
		ReferenceVariant: map[string]string{"size": "M", "color": "white"},
		DesignURL:        "https://placehold.co/4000x5000/png?text=Design+Placeholder",
		MarginPct:        40,
	},
	"mug": {
		GelatoCatalog: "mugs",
		Name:          "Ceramic Mug",
		Description:   "White ceramic, printed wrap",
		FixedFilters: map[string]string{
			"MugMaterial":   "ceramic-white",
			"ProductStatus": "activated",
		},
		AgentOptions:     map[string]string{"size": "MugSize"},
		ReferenceVariant: map[string]string{"size": "11-oz"},
		DesignURL:        "https://placehold.co/4000x2000/png?text=Design+Placeholder",
		MarginPct:        40,
	},
	"framed-print": {
		GelatoCatalog: "framed-posters",
		Name:          "Framed Print",
		Description:   "Black wood frame, plexiglass front",
		FixedFilters: map[string]string{
			"FrameColor":    "black",
			"FrameMaterial": "wood",
			"ProductStatus": "activated",
		},
		AgentOptions:     map[string]string{"size": "PaperFormat", "orientation": "Orientation"},
		ReferenceVariant: map[string]string{"size": "a3", "orientation": "ver"},
		DesignURL:        "https://placehold.co/3000x4000/png?text=Design+Placeholder",
		MarginPct:        40,
	},
}

// ProductOrder preserves display order.
var ProductOrder = []string{"t-shirt", "mug", "framed-print"}

// Apparel size sort order.
var apparelSizeOrder = map[string]int{
	"XS": 0, "S": 1, "M": 2, "L": 3, "XL": 4,
	"2XL": 5, "3XL": 6, "4XL": 7, "5XL": 8,
}

// --- TTL cache ---

type cacheEntry struct {
	data      interface{}
	fetchedAt time.Time
}

var (
	cacheMu sync.RWMutex
	cache   = map[string]cacheEntry{}
)

func getCached(key string, ttl int, fetchFn func() (interface{}, error)) (interface{}, error) {
	cacheMu.RLock()
	entry, ok := cache[key]
	cacheMu.RUnlock()

	if ok && time.Since(entry.fetchedAt).Seconds() < float64(ttl) {
		return entry.data, nil
	}

	data, err := fetchFn()
	if err != nil {
		// Return stale on error
		if ok {
			return entry.data, nil
		}
		return nil, err
	}

	cacheMu.Lock()
	cache[key] = cacheEntry{data: data, fetchedAt: time.Now()}
	cacheMu.Unlock()
	return data, nil
}

func gelatoHeaders() http.Header {
	h := http.Header{}
	h.Set("X-API-KEY", os.Getenv("GELATO_API_KEY"))
	h.Set("Content-Type", "application/json")
	return h
}

// --- Raw Gelato API calls ---

func fetchValidOptions(productID string) (map[string][]string, error) {
	cfg, ok := CatalogConfig[productID]
	if !ok {
		return nil, fmt.Errorf("unknown product: %s", productID)
	}

	attrFilters := map[string][]string{}
	for k, v := range cfg.FixedFilters {
		attrFilters[k] = []string{v}
	}

	type gelatoProduct struct {
		ProductUID string            `json:"productUid"`
		Attributes map[string]string `json:"attributes"`
	}

	var allProducts []gelatoProduct
	offset := 0
	client := &http.Client{Timeout: 15 * time.Second}

	for {
		payload, _ := json.Marshal(map[string]interface{}{
			"attributeFilters": attrFilters,
			"limit":            100,
			"offset":           offset,
		})

		req, _ := http.NewRequest("POST",
			fmt.Sprintf("%s/catalogs/%s/products:search", gelatoCatalogURL, cfg.GelatoCatalog),
			bytes.NewReader(payload))
		req.Header = gelatoHeaders()

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("Gelato API error: %d", resp.StatusCode)
		}

		var result struct {
			Products []gelatoProduct `json:"products"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		if len(result.Products) == 0 {
			break
		}
		allProducts = append(allProducts, result.Products...)
		offset += len(result.Products)
	}

	options := map[string][]string{}
	for optName, gelatoAttr := range cfg.AgentOptions {
		seen := map[string]bool{}
		for _, p := range allProducts {
			if v := p.Attributes[gelatoAttr]; v != "" {
				seen[v] = true
			}
		}
		vals := make([]string, 0, len(seen))
		for v := range seen {
			vals = append(vals, v)
		}
		sort.Strings(vals)
		// Sort apparel sizes in standard order
		if gelatoAttr == "GarmentSize" {
			sort.Slice(vals, func(i, j int) bool {
				oi, oki := apparelSizeOrder[vals[i]]
				oj, okj := apparelSizeOrder[vals[j]]
				if !oki {
					oi = 99
				}
				if !okj {
					oj = 99
				}
				return oi < oj
			})
		}
		options[optName] = vals
	}

	return options, nil
}

func searchGelatoProduct(catalogUID string, filters map[string]string) (string, error) {
	attrFilters := map[string][]string{}
	for k, v := range filters {
		attrFilters[k] = []string{v}
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"attributeFilters": attrFilters,
		"limit":            1,
	})

	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/catalogs/%s/products:search", gelatoCatalogURL, catalogUID),
		bytes.NewReader(payload))
	req.Header = gelatoHeaders()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Gelato search error: %d", resp.StatusCode)
	}

	var result struct {
		Products []struct {
			ProductUID string `json:"productUid"`
		} `json:"products"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Products) == 0 {
		return "", nil
	}
	return result.Products[0].ProductUID, nil
}

func fetchProductPriceUSD(productUID string) (float64, error) {
	req, _ := http.NewRequest("GET",
		fmt.Sprintf("%s/products/%s/prices", gelatoCatalogURL, productUID), nil)
	req.Header = gelatoHeaders()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("Gelato price error: %d", resp.StatusCode)
	}

	var prices []struct {
		Price float64 `json:"price"`
	}
	if err := json.Unmarshal(body, &prices); err != nil {
		// Try single object
		var single struct {
			Price float64 `json:"price"`
		}
		if err2 := json.Unmarshal(body, &single); err2 != nil {
			return 0, err
		}
		return single.Price, nil
	}
	if len(prices) == 0 {
		return 0, fmt.Errorf("no price data")
	}
	return prices[0].Price, nil
}

func fetchBCHRate() (float64, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(coingeckoURL + "?ids=bitcoin-cash&vs_currencies=usd")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data map[string]map[string]float64
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}
	if bch, ok := data["bitcoin-cash"]; ok {
		if usd, ok := bch["usd"]; ok {
			return usd, nil
		}
	}
	return 0, fmt.Errorf("BCH rate not found in response")
}

// --- Public API ---

func GetProduct(productID string) *ProductConfig {
	cfg, ok := CatalogConfig[productID]
	if !ok {
		return nil
	}
	return &cfg
}

func GetProductOptions(productID string) (map[string][]string, error) {
	if _, ok := CatalogConfig[productID]; !ok {
		return nil, fmt.Errorf("unknown product: %s", productID)
	}
	key := "valid_options:" + productID
	data, err := getCached(key, catalogTTL, func() (interface{}, error) {
		return fetchValidOptions(productID)
	})
	if err != nil {
		return nil, err
	}
	return data.(map[string][]string), nil
}

func ValidateOptions(productID string, options map[string]string) string {
	cfg, ok := CatalogConfig[productID]
	if !ok {
		return fmt.Sprintf("Unknown product: %s", productID)
	}
	for optName := range cfg.AgentOptions {
		if _, has := options[optName]; !has {
			return fmt.Sprintf("Missing required option '%s'", optName)
		}
	}
	for optName := range options {
		if _, has := cfg.AgentOptions[optName]; !has {
			keys := make([]string, 0, len(cfg.AgentOptions))
			for k := range cfg.AgentOptions {
				keys = append(keys, k)
			}
			return fmt.Sprintf("Unknown option '%s'. Valid options: %v", optName, keys)
		}
	}
	return ""
}

func ResolveGelatoUID(productID string, agentChoices map[string]string) (string, error) {
	cfg, ok := CatalogConfig[productID]
	if !ok {
		return "", fmt.Errorf("unknown product: %s", productID)
	}

	filters := map[string]string{}
	for k, v := range cfg.FixedFilters {
		filters[k] = v
	}
	for optName, optValue := range agentChoices {
		if gelatoAttr, ok := cfg.AgentOptions[optName]; ok {
			filters[gelatoAttr] = optValue
		}
	}

	cacheKey := fmt.Sprintf("uid:%s:%v", productID, agentChoices)
	data, err := getCached(cacheKey, catalogTTL, func() (interface{}, error) {
		uid, err := searchGelatoProduct(cfg.GelatoCatalog, filters)
		if err != nil {
			return nil, err
		}
		return uid, nil
	})
	if err != nil {
		return "", err
	}
	return data.(string), nil
}

func GetProductBCHPrice(productID string, agentChoices map[string]string) (string, error) {
	cfg, ok := CatalogConfig[productID]
	if !ok {
		return "", fmt.Errorf("unknown product: %s", productID)
	}

	choices := agentChoices
	if choices == nil {
		choices = cfg.ReferenceVariant
	}

	uid, err := ResolveGelatoUID(productID, choices)
	if err != nil || uid == "" {
		return "", fmt.Errorf("could not resolve product UID")
	}

	// Get USD cost
	priceData, err := getCached("price_usd:"+uid, priceTTL, func() (interface{}, error) {
		return fetchProductPriceUSD(uid)
	})
	if err != nil {
		return "", err
	}
	usdCost := priceData.(float64)

	// Get BCH rate
	rateData, err := getCached("bch_rate", rateTTL, func() (interface{}, error) {
		return fetchBCHRate()
	})
	if err != nil {
		return "", err
	}
	bchRate := rateData.(float64)

	usdWithMargin := usdCost * (1 + cfg.MarginPct/100)
	bch := usdWithMargin / bchRate
	return fmt.Sprintf("%.6f", bch), nil
}

func GetProductsForMenu() []MenuItem {
	apiKey := os.Getenv("GELATO_API_KEY")
	if apiKey == "" {
		items := make([]MenuItem, 0, len(ProductOrder))
		for _, pid := range ProductOrder {
			cfg := CatalogConfig[pid]
			items = append(items, MenuItem{
				ID:           pid,
				Name:         cfg.Name + " — " + cfg.Description,
				Available:    false,
				BasePriceBCH: "0.000000",
			})
		}
		return items
	}

	// Fetch prices concurrently
	type result struct {
		pid   string
		price string
		err   error
	}
	ch := make(chan result, len(ProductOrder))
	for _, pid := range ProductOrder {
		go func(p string) {
			price, err := GetProductBCHPrice(p, nil)
			ch <- result{pid: p, price: price, err: err}
		}(pid)
	}

	prices := map[string]string{}
	for range ProductOrder {
		r := <-ch
		if r.err == nil && r.price != "" {
			prices[r.pid] = r.price
		}
	}

	items := make([]MenuItem, 0, len(ProductOrder))
	for _, pid := range ProductOrder {
		cfg := CatalogConfig[pid]
		price, ok := prices[pid]
		items = append(items, MenuItem{
			ID:           pid,
			Name:         cfg.Name + " — " + cfg.Description,
			Available:    ok,
			BasePriceBCH: orDefault(price, "0.000000"),
		})
	}
	return items
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
