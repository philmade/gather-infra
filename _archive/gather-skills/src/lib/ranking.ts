import type { RankingWeights } from './types.js';

const DEFAULT_WEIGHTS: RankingWeights = {
  reviews: 0.40,
  installs: 0.25,
  proofs: 0.35,
};

export function calculateRankScore(
  avgScore: number | null,
  reviewCount: number,
  installs: number,
  proofCount: number,
  totalReviews: number,
  weights: RankingWeights = DEFAULT_WEIGHTS
): number {
  if (avgScore === null || reviewCount === 0) {
    return 0;
  }

  // Normalize review count (log scale to prevent dominance by very popular skills)
  const normalizedReviewCount = Math.log10(reviewCount + 1) / Math.log10(totalReviews + 10);

  // Normalize installs (log scale)
  const normalizedInstalls = Math.log10(installs + 1) / Math.log10(10000);

  // Proof ratio: what percentage of reviews have proofs
  const proofRatio = reviewCount > 0 ? proofCount / reviewCount : 0;

  // Calculate weighted score
  const score =
    (weights.reviews * avgScore * normalizedReviewCount) +
    (weights.installs * normalizedInstalls) +
    (weights.proofs * proofRatio * avgScore);

  // Return normalized to 0-100 scale
  return Math.min(100, Math.max(0, score * 10));
}

export function updateSkillRanking(
  db: import('better-sqlite3').Database,
  skillId: string
): void {
  // Get skill stats
  const skill = db.prepare(`
    SELECT
      s.avg_score,
      s.review_count,
      s.installs,
      (SELECT COUNT(*) FROM proofs p JOIN reviews r ON p.review_id = r.id WHERE r.skill_id = s.id AND p.verified = 1) as proof_count
    FROM skills s
    WHERE s.id = ?
  `).get(skillId) as { avg_score: number | null; review_count: number; installs: number; proof_count: number } | undefined;

  if (!skill) return;

  // Get total reviews for normalization
  const totalReviews = (db.prepare('SELECT SUM(review_count) as total FROM skills').get() as { total: number })?.total || 0;

  const rankScore = calculateRankScore(
    skill.avg_score,
    skill.review_count,
    skill.installs,
    skill.proof_count,
    totalReviews
  );

  db.prepare('UPDATE skills SET rank_score = ? WHERE id = ?').run(rankScore, skillId);
}

export function updateAllRankings(db: import('better-sqlite3').Database): void {
  const skills = db.prepare('SELECT id FROM skills').all() as Array<{ id: string }>;
  for (const skill of skills) {
    updateSkillRanking(db, skill.id);
  }
}
