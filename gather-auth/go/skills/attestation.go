package skills

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	auth "gather.is/auth"
)

// AttestationPayload is the data that gets hashed and signed.
type AttestationPayload struct {
	SkillID    string   `json:"skill_id"`
	TaskHash   string   `json:"task_hash"`
	OutputHash string   `json:"output_hash"`
	Score      *float64 `json:"score"`
	Timestamp  int64    `json:"timestamp"`
}

// Attestation is a cryptographic proof of execution.
type Attestation struct {
	ID            string             `json:"id"`
	Version       string             `json:"version"`
	ExecutionHash string             `json:"execution_hash"`
	Payload       AttestationPayload `json:"payload"`
	Signature     string             `json:"signature"`
	PublicKey     string             `json:"public_key"`
}

// ExecutionData is the input for creating an attestation.
type ExecutionData struct {
	SkillID         string
	Task            string
	CLIOutput       string
	Score           *float64
	WhatWorked      string
	WhatFailed      string
	ExecutionTimeMs *float64
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// CreateAttestation generates a signed proof using the server's Ed25519 keypair.
// The keypair is loaded from ~/.gather/keys/server.
func CreateAttestation(data ExecutionData) (*Attestation, error) {
	kp, err := auth.LoadKeyPair("server")
	if err != nil {
		// Generate one if it doesn't exist
		kp, err = auth.GenerateKeyPair()
		if err != nil {
			return nil, fmt.Errorf("generate keypair: %w", err)
		}
		if err := auth.SaveKeyPair("server", kp); err != nil {
			return nil, fmt.Errorf("save keypair: %w", err)
		}
	}

	timestamp := int64(0) // Will use record creation time instead
	payload := AttestationPayload{
		SkillID:    data.SkillID,
		TaskHash:   hashContent(data.Task),
		OutputHash: hashContent(data.CLIOutput),
		Score:      data.Score,
		Timestamp:  timestamp,
	}

	// Create execution hash from all relevant data
	execData := map[string]interface{}{
		"skill_id":    payload.SkillID,
		"task_hash":   payload.TaskHash,
		"output_hash": payload.OutputHash,
		"score":       payload.Score,
		"timestamp":   payload.Timestamp,
		"what_worked": data.WhatWorked,
		"what_failed": data.WhatFailed,
	}
	if data.ExecutionTimeMs != nil {
		execData["execution_time_ms"] = *data.ExecutionTimeMs
	}
	execJSON, _ := json.Marshal(execData)
	executionHash := hashContent(string(execJSON))

	// Sign the execution hash
	sig := ed25519.Sign(kp.PrivateKey, []byte(executionHash))

	pubPEM, err := auth.EncodePEM(kp.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("encode public key: %w", err)
	}

	return &Attestation{
		ID:            uuid.New().String(),
		Version:       "1.0.0",
		ExecutionHash: executionHash,
		Payload:       payload,
		Signature:     base64.StdEncoding.EncodeToString(sig),
		PublicKey:     string(pubPEM),
	}, nil
}

// VerifyAttestation checks an Ed25519 signature on an attestation.
func VerifyAttestation(executionHash, signatureB64, publicKeyPEM string) bool {
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false
	}

	pubKey, err := auth.ParsePublicKeyPEM([]byte(publicKeyPEM))
	if err != nil {
		return false
	}

	return ed25519.Verify(pubKey, []byte(executionHash), sig)
}
