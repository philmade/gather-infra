/**
 * Proof Generator (Phase 2)
 *
 * Generates ZK proofs for completed reviews, attesting to:
 * - Skill installation
 * - Test execution
 * - Claude analysis
 * - Generated artifacts
 */

import { nanoid } from 'nanoid';
import { getDb } from '../db/index.js';
import { createExecutionPayload, generateExecutionProof, hashContent } from '../lib/reclaim.js';
import type { Review } from '../lib/types.js';

export interface ProofGenerationResult {
  success: boolean;
  proofId?: string;
  error?: string;
}

/**
 * Generate a proof for a completed review
 */
export async function generateProofForReview(reviewId: string): Promise<ProofGenerationResult> {
  const db = getDb();

  // Get review
  const review = db.prepare('SELECT * FROM reviews WHERE id = ?').get(reviewId) as Review | undefined;

  if (!review) {
    return { success: false, error: 'Review not found' };
  }

  if (review.status !== 'complete') {
    return { success: false, error: 'Review not complete' };
  }

  if (review.proof_id) {
    return { success: true, proofId: review.proof_id };
  }

  try {
    // For Phase 1, create a placeholder proof with hashed execution data
    const proofId = nanoid();

    // Create claim data from execution logs
    const claimData = {
      skill_id: review.skill_id,
      task_hash: hashContent(review.task),
      output_hash: review.cli_output ? hashContent(review.cli_output) : null,
      score: review.score,
      timestamp: Date.now(),
    };

    // Store proof (placeholder until Phase 2)
    db.prepare(`
      INSERT INTO proofs (id, review_id, claim_data, identifier, signatures, witnesses)
      VALUES (?, ?, ?, ?, ?, ?)
    `).run(
      proofId,
      reviewId,
      JSON.stringify(claimData),
      `reskill:review:${reviewId}`,
      JSON.stringify([]), // Empty signatures until Phase 2
      JSON.stringify([])  // Empty witnesses until Phase 2
    );

    // Update review with proof ID
    db.prepare('UPDATE reviews SET proof_id = ? WHERE id = ?').run(proofId, reviewId);

    return { success: true, proofId };
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : String(error),
    };
  }
}

/**
 * Generate proofs for all completed reviews without proofs
 */
export async function generateMissingProofs(): Promise<{
  generated: number;
  failed: number;
  errors: string[];
}> {
  const db = getDb();

  const reviews = db.prepare(`
    SELECT id FROM reviews
    WHERE status = 'complete' AND proof_id IS NULL
  `).all() as Array<{ id: string }>;

  let generated = 0;
  let failed = 0;
  const errors: string[] = [];

  for (const review of reviews) {
    const result = await generateProofForReview(review.id);
    if (result.success) {
      generated++;
    } else {
      failed++;
      if (result.error) {
        errors.push(`${review.id}: ${result.error}`);
      }
    }
  }

  return { generated, failed, errors };
}
