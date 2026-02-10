import { Router } from 'express';
import { getDb } from '../../db/index.js';
import { updateAllRankings } from '../../lib/ranking.js';

const router = Router();

// GET /api/rankings - Get ranked skills with proof counts
router.get('/', (req, res) => {
  const db = getDb();
  const limit = Math.min(100, parseInt(req.query.limit as string) || 20);

  const rankings = db.prepare(`
    SELECT
      s.id,
      s.name,
      s.description,
      s.installs,
      s.review_count,
      s.avg_score,
      s.rank_score,
      (SELECT COUNT(*) FROM proofs p JOIN reviews r ON p.review_id = r.id WHERE r.skill_id = s.id AND p.verified = 1) as verified_proofs
    FROM skills s
    WHERE s.review_count > 0
    ORDER BY s.rank_score DESC NULLS LAST
    LIMIT ?
  `).all(limit) as Array<{
    id: string;
    name: string;
    description: string | null;
    installs: number;
    review_count: number;
    avg_score: number | null;
    rank_score: number | null;
    verified_proofs: number;
  }>;

  res.json({
    rankings,
    count: rankings.length,
  });
});

// POST /api/rankings/refresh - Recalculate all rankings
router.post('/refresh', (req, res) => {
  const db = getDb();
  updateAllRankings(db);

  res.json({ message: 'Rankings refreshed' });
});

export default router;
