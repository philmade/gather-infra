#!/bin/bash
set -e

echo "╔════════════════════════════════════════════════════════════╗"
echo "║          RESKILL FULL E2E TEST                             ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Start server
RESKILL_MOCK=1 npm run dev &
SERVER_PID=$!
sleep 2

cleanup() {
  kill $SERVER_PID 2>/dev/null || true
  pkill -f "tsx watch" 2>/dev/null || true
}
trap cleanup EXIT

echo "═══════════════════════════════════════════════════════════"
echo "STEP 1: Register Agent"
echo "═══════════════════════════════════════════════════════════"
REG_RESPONSE=$(curl -s -X POST http://localhost:3000/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name": "E2E-TestBot", "description": "End-to-end test agent"}')

API_KEY=$(echo "$REG_RESPONSE" | jq -r '.api_key')
CODE=$(echo "$REG_RESPONSE" | jq -r '.verification.code')
AGENT_ID=$(echo "$REG_RESPONSE" | jq -r '.agent.id')

echo "Agent ID: $AGENT_ID"
echo "API Key: ${API_KEY:0:25}..."
echo "Verification Code: $CODE"
echo "Tweet Template: $(echo "$REG_RESPONSE" | jq -r '.verification.tweet_template')"
echo ""

echo "═══════════════════════════════════════════════════════════"
echo "STEP 2: Verify Agent via Tweet (mock)"
echo "═══════════════════════════════════════════════════════════"
MOCK_TWEET="mock://twitter.com/e2e_tester/status/12345?code=$CODE"
echo "Mock tweet URL: $MOCK_TWEET"

VERIFY_RESPONSE=$(curl -s -X POST http://localhost:3000/api/auth/verify \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: $API_KEY" \
  -d "{\"tweet_url\": \"$MOCK_TWEET\"}")

echo "$VERIFY_RESPONSE" | jq '.'
echo ""

echo "═══════════════════════════════════════════════════════════"
echo "STEP 3: Check Agent Status (should be verified)"
echo "═══════════════════════════════════════════════════════════"
curl -s http://localhost:3000/api/auth/me -H "X-Api-Key: $API_KEY" | jq '{name, twitter_handle, twitter_verified}'
echo ""

echo "═══════════════════════════════════════════════════════════"
echo "STEP 4: Create Review (authenticated)"
echo "═══════════════════════════════════════════════════════════"
REVIEW_RESPONSE=$(curl -s -X POST http://localhost:3000/api/reviews \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: $API_KEY" \
  -d '{"skill_id": "e2e-full/test-skill", "task": "Full E2E test of the review system"}')

REVIEW_ID=$(echo "$REVIEW_RESPONSE" | jq -r '.id')
echo "Review ID: $REVIEW_ID"
echo "Status: $(echo "$REVIEW_RESPONSE" | jq -r '.status')"
echo ""

# Wait for review to complete
echo "Waiting for review to complete..."
sleep 3

echo "═══════════════════════════════════════════════════════════"
echo "STEP 5: Check Review Result"
echo "═══════════════════════════════════════════════════════════"
REVIEW_RESULT=$(curl -s "http://localhost:3000/api/reviews/$REVIEW_ID")
echo "$REVIEW_RESULT" | jq '{skill_id, agent_id, status, score, proof_id: .proof.id}'
echo ""

PROOF_ID=$(echo "$REVIEW_RESULT" | jq -r '.proof.id')

echo "═══════════════════════════════════════════════════════════"
echo "STEP 6: Verify Proof"
echo "═══════════════════════════════════════════════════════════"
if [ "$PROOF_ID" != "null" ]; then
  curl -s -X POST "http://localhost:3000/api/proofs/$PROOF_ID/verify" | jq '.'
else
  echo "No proof generated (review may have failed)"
fi
echo ""

echo "═══════════════════════════════════════════════════════════"
echo "STEP 7: Check Rankings"
echo "═══════════════════════════════════════════════════════════"
curl -s http://localhost:3000/api/rankings | jq '.rankings[:5] | .[] | {name, avg_score, verified_proofs}'
echo ""

echo "═══════════════════════════════════════════════════════════"
echo "STEP 8: Database Summary"
echo "═══════════════════════════════════════════════════════════"
echo "Agents:"
sqlite3 data/reskill.db "SELECT name, twitter_handle, twitter_verified FROM agents WHERE name = 'E2E-TestBot';"
echo ""
echo "Review:"
sqlite3 data/reskill.db "SELECT r.id, r.status, r.score, a.name as agent FROM reviews r LEFT JOIN agents a ON r.agent_id = a.id WHERE r.skill_id = 'e2e-full/test-skill' ORDER BY r.created_at DESC LIMIT 1;"
echo ""
echo "Proof:"
sqlite3 data/reskill.db "SELECT p.id, p.verified FROM proofs p JOIN reviews r ON p.review_id = r.id WHERE r.skill_id = 'e2e-full/test-skill' ORDER BY p.created_at DESC LIMIT 1;"

echo ""
echo "╔════════════════════════════════════════════════════════════╗"
echo "║          ✓ E2E TEST COMPLETE                               ║"
echo "╚════════════════════════════════════════════════════════════╝"
