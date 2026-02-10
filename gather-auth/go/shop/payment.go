package shop

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"time"
)


var txIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

const blockchairURL = "https://api.blockchair.com/bitcoin-cash/dashboards/transaction"

var satPerBCH = big.NewInt(100_000_000)

func ShopBCHAddress() string {
	addr := os.Getenv("BCH_ADDRESS")
	if addr == "" {
		addr = "YOUR_BCH_ADDRESS"
	}
	return addr
}

func stripPrefix(address string) string {
	if len(address) > 12 && address[:12] == "bitcoincash:" {
		return address[12:]
	}
	return address
}

// VerifyTransaction checks a BCH transaction via Blockchair.
// Returns (ok, message).
func VerifyTransaction(txID string, expectedBCH string) (bool, string) {
	if !txIDPattern.MatchString(txID) {
		return false, fmt.Sprintf(
			"Invalid transaction ID format. Expected a 64-character lowercase hex string. "+
				"Received: '%s' (%d chars).", txID, len(txID))
	}

	expectedRat := new(big.Rat)
	if _, ok := expectedRat.SetString(expectedBCH); !ok {
		return false, "Invalid expected amount."
	}
	// expectedSats = expectedBCH * 100_000_000
	expectedSats := new(big.Int)
	expectedSats.Mul(expectedRat.Num(), satPerBCH)
	expectedSats.Div(expectedSats, expectedRat.Denom())

	shopAddr := stripPrefix(ShopBCHAddress())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(blockchairURL + "/" + txID)
	if err != nil {
		return false, "Payment verification service unavailable. Please try again."
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, "Payment verification service returned an error. Please try again."
	}

	var result struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "Failed to parse blockchain response."
	}

	txRaw, ok := result.Data[txID]
	if !ok {
		return false, fmt.Sprintf("Transaction %s not found on the BCH blockchain.", txID)
	}

	var txData struct {
		Outputs []struct {
			Recipient string `json:"recipient"`
			Value     int64  `json:"value"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(txRaw, &txData); err != nil {
		return false, "Failed to parse transaction data."
	}

	for _, out := range txData.Outputs {
		if out.Recipient == shopAddr {
			if big.NewInt(out.Value).Cmp(expectedSats) >= 0 {
				return true, "Payment verified on blockchain."
			}
			actualRat := new(big.Rat).SetFrac(big.NewInt(out.Value), satPerBCH)
			return false, fmt.Sprintf(
				"Payment amount insufficient. Expected >= %s BCH, found %s BCH.",
				expectedBCH, actualRat.FloatString(8))
		}
	}

	return false, fmt.Sprintf(
		"Transaction does not include payment to shop address (%s). "+
			"Please send BCH to the payment_address returned in your order.",
		ShopBCHAddress())
}

