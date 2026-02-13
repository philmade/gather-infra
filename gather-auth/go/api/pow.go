package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"

	"gather.is/auth/hashcash"
)

// -----------------------------------------------------------------------------
// PoW challenge store (in-memory, single-use, TTL-based)
// -----------------------------------------------------------------------------

const (
	powChallengeTTL     = 5 * time.Minute
	powCleanupInterval  = 1 * time.Minute
	defaultRegDifficulty  = 22 // ~2-5 seconds
	defaultPostDifficulty = 22 // ~2-5 seconds
)

type powEntry struct {
	Challenge  string
	Purpose    string
	Difficulty int
	CreatedAt  time.Time
}

type PowStore struct {
	mu    sync.Mutex
	items map[string]*powEntry // keyed by challenge string
}

func NewPowStore() *PowStore {
	ps := &PowStore{items: make(map[string]*powEntry)}
	go ps.cleanup()
	return ps
}

func (ps *PowStore) Add(challenge, purpose string, difficulty int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.items[challenge] = &powEntry{
		Challenge:  challenge,
		Purpose:    purpose,
		Difficulty: difficulty,
		CreatedAt:  time.Now(),
	}
}

// Consume retrieves and deletes a challenge. Returns nil if not found or expired.
func (ps *PowStore) Consume(challenge, purpose string) *powEntry {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	entry, ok := ps.items[challenge]
	if !ok {
		return nil
	}
	delete(ps.items, challenge)
	if time.Since(entry.CreatedAt) > powChallengeTTL {
		return nil // expired
	}
	if entry.Purpose != purpose {
		return nil // wrong purpose
	}
	return entry
}

func (ps *PowStore) cleanup() {
	for {
		time.Sleep(powCleanupInterval)
		ps.mu.Lock()
		now := time.Now()
		for k, v := range ps.items {
			if now.Sub(v.CreatedAt) > powChallengeTTL {
				delete(ps.items, k)
			}
		}
		ps.mu.Unlock()
	}
}

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type PowChallengeInput struct {
	Body struct {
		Purpose string `json:"purpose" doc:"What the proof-of-work is for: 'register' or 'post'" minLength:"1"`
	}
}

type PowChallengeOutput struct {
	Body struct {
		Challenge  string `json:"challenge" doc:"Challenge string — find a nonce where SHA-256(challenge + ':' + nonce) has leading zero bits"`
		Difficulty int    `json:"difficulty" doc:"Required number of leading zero bits in the hash"`
		Algorithm  string `json:"algorithm" doc:"Always sha256"`
		ExpiresIn  int    `json:"expires_in" doc:"Seconds until challenge expires"`
		Hint       string `json:"hint" doc:"How to solve this"`
	}
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterPowRoutes(api huma.API, app *pocketbase.PocketBase, ps *PowStore) {
	huma.Register(api, huma.Operation{
		OperationID: "pow-challenge",
		Method:      "POST",
		Path:        "/api/pow/challenge",
		Summary:     "Get a proof-of-work challenge",
		Description: "Returns a challenge that must be solved before registering or posting. " +
			"Find a nonce where SHA-256(challenge + ':' + nonce) has the required number of leading zero bits. " +
			"This prevents spam by requiring a few seconds of computation per action.",
		Tags: []string{"Proof of Work"},
	}, func(ctx context.Context, input *PowChallengeInput) (*PowChallengeOutput, error) {
		purpose := input.Body.Purpose
		if purpose != "register" && purpose != "post" {
			return nil, huma.Error422UnprocessableEntity("purpose must be 'register' or 'post'")
		}

		difficulty := powDifficulty(app, purpose)

		challenge, err := hashcash.NewChallenge()
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to generate challenge")
		}

		ps.Add(challenge, purpose, difficulty)

		out := &PowChallengeOutput{}
		out.Body.Challenge = challenge
		out.Body.Difficulty = difficulty
		out.Body.Algorithm = "sha256"
		out.Body.ExpiresIn = int(powChallengeTTL.Seconds())
		out.Body.Hint = fmt.Sprintf(
			"Find a nonce (string) where SHA-256(\"%s:\" + nonce) has %d leading zero bits. "+
				"Iterate nonces (\"0\", \"1\", \"2\", ...) and hash until you find one. This takes a few seconds.",
			challenge, difficulty)
		return out, nil
	})
}

// powDifficulty reads difficulty from platform_config, with sensible defaults.
func powDifficulty(app *pocketbase.PocketBase, purpose string) int {
	field := "pow_difficulty_" + purpose // e.g. pow_difficulty_register, pow_difficulty_post
	records, err := app.FindRecordsByFilter("platform_config", "id != ''", "", 1, 0, nil)
	if err == nil && len(records) > 0 {
		v := int(records[0].GetFloat(field))
		if v > 0 {
			return v
		}
	}
	switch purpose {
	case "register":
		return defaultRegDifficulty
	default:
		return defaultPostDifficulty
	}
}

// VerifyPow checks a PoW solution against the store. Returns an error suitable for API responses.
func VerifyPow(ps *PowStore, challenge, nonce, purpose string) error {
	if challenge == "" || nonce == "" {
		return fmt.Errorf("proof-of-work required: call POST /api/pow/challenge with purpose '%s', solve it, then include pow_challenge and pow_nonce in your request", purpose)
	}

	entry := ps.Consume(challenge, purpose)
	if entry == nil {
		return fmt.Errorf("invalid, expired, or already-used proof-of-work challenge — request a new one via POST /api/pow/challenge")
	}

	if !hashcash.Verify(challenge, nonce, entry.Difficulty) {
		return fmt.Errorf("proof-of-work verification failed: SHA-256(%s:%s) does not have %d leading zero bits", challenge, nonce, entry.Difficulty)
	}

	return nil
}
