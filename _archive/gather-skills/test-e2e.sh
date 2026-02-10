#!/bin/bash
set -e

echo "=== Reskill E2E Test ==="
echo ""

# Clean slate - remove old test data
echo "1. Cleaning test data..."
sqlite3 data/reskill.db "DELETE FROM proofs WHERE review_id IN (SELECT id FROM reviews WHERE skill_id LIKE 'e2e-test/%');" 2>/dev/null || true
sqlite3 data/reskill.db "DELETE FROM reviews WHERE skill_id LIKE 'e2e-test/%';" 2>/dev/null || true
sqlite3 data/reskill.db "DELETE FROM skills WHERE id LIKE 'e2e-test/%';" 2>/dev/null || true
echo "   ✓ Cleaned"

# Run 3 mock reviews
echo ""
echo "2. Running mock reviews..."
for i in 1 2 3; do
  RESKILL_MOCK=1 npx tsx src/cli/index.ts review "e2e-test/skill-$i" --task "Test task $i" --wait > /dev/null 2>&1
  echo "   ✓ Review $i complete"
done

# Check skills created
echo ""
echo "3. Checking skills in database..."
sqlite3 data/reskill.db "SELECT id, name, review_count, avg_score FROM skills WHERE id LIKE 'e2e-test/%';"

# Check reviews created
echo ""
echo "4. Checking reviews in database..."
sqlite3 data/reskill.db "SELECT id, skill_id, status, score, proof_id IS NOT NULL as has_proof FROM reviews WHERE skill_id LIKE 'e2e-test/%';"

# Check proofs created
echo ""
echo "5. Checking proofs in database..."
sqlite3 data/reskill.db "SELECT p.id, p.verified, substr(p.identifier, 1, 16) || '...' as hash FROM proofs p JOIN reviews r ON p.review_id = r.id WHERE r.skill_id LIKE 'e2e-test/%';"

# Verify a proof
echo ""
echo "6. Verifying proof signature..."
REVIEW_ID=$(sqlite3 data/reskill.db "SELECT id FROM reviews WHERE skill_id LIKE 'e2e-test/%' LIMIT 1;")
npx tsx src/cli/index.ts proof "$REVIEW_ID" --verify 2>&1 | grep -E "(Verifying|verified|failed)"

# Check rankings
echo ""
echo "7. Checking rankings..."
npx tsx src/cli/index.ts list 2>&1 | head -15

echo ""
echo "=== E2E Test Complete ==="
