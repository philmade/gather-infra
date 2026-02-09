import { Router } from 'express';
import { nanoid } from 'nanoid';
import { getDb } from '../../db/index.js';
import type { Skill } from '../../lib/types.js';

const router = Router();

// Sort options mapped to safe SQL fragments (no injection)
const SORT_MAP: Record<string, string> = {
  rank: 'rank_score DESC NULLS LAST, review_count DESC',
  installs: 'installs DESC NULLS LAST',
  reviews: 'review_count DESC',
  security: 'avg_security_score DESC NULLS LAST',
  newest: 'created_at DESC',
};

// GET /api/skills - List skills sorted by rank, with optional search/filter
router.get('/', (req, res) => {
  const db = getDb();
  const limit = Math.max(1, Math.min(100, parseInt(req.query.limit as string) || 50));
  const offset = Math.max(0, parseInt(req.query.offset as string) || 0);
  const q = (req.query.q as string || '').trim();
  const category = (req.query.category as string || '').trim();
  const sortKey = (req.query.sort as string || 'rank');
  const minSecurity = parseFloat(req.query.min_security as string);

  const conditions: string[] = [];
  const params: (string | number)[] = [];

  if (q) {
    conditions.push('(name LIKE ? OR description LIKE ? OR id LIKE ?)');
    const pattern = `%${q}%`;
    params.push(pattern, pattern, pattern);
  }

  if (category) {
    conditions.push('category = ?');
    params.push(category);
  }

  if (!isNaN(minSecurity)) {
    conditions.push('avg_security_score >= ?');
    params.push(minSecurity);
  }

  const whereClause = conditions.length > 0 ? `WHERE ${conditions.join(' AND ')}` : '';
  const orderBy = SORT_MAP[sortKey] || SORT_MAP.rank;

  const skills = db.prepare(`
    SELECT * FROM skills
    ${whereClause}
    ORDER BY ${orderBy}, created_at DESC
    LIMIT ? OFFSET ?
  `).all(...params, limit, offset) as Skill[];

  const total = (db.prepare(`SELECT COUNT(*) as count FROM skills ${whereClause}`).get(...params) as { count: number }).count;

  res.json({
    skills,
    total,
    limit,
    offset,
  });
});

// GET /api/skills/:id - Skill details with reviews (wildcard for slash-containing IDs like "anthropics/pdf")
router.get('/:id(*)', (req, res) => {
  const db = getDb();
  const skillId = req.params.id;

  const skill = db.prepare('SELECT * FROM skills WHERE id = ?').get(skillId) as Skill | undefined;

  if (!skill) {
    return res.status(404).json({ error: 'Skill not found' });
  }

  const reviews = db.prepare(`
    SELECT id, task, status, score, what_worked, what_failed, skill_feedback, agent_model, execution_time_ms, created_at
    FROM reviews
    WHERE skill_id = ?
    ORDER BY created_at DESC
    LIMIT 20
  `).all(skillId);

  res.json({
    ...skill,
    reviews,
  });
});

// POST /api/skills - Add a skill
router.post('/', (req, res) => {
  const db = getDb();
  const { id, name, description, source, category } = req.body;

  if (!id || !name) {
    return res.status(400).json({ error: 'id and name are required' });
  }

  // Check if skill already exists
  const existing = db.prepare('SELECT id FROM skills WHERE id = ?').get(id);
  if (existing) {
    return res.status(409).json({ error: 'Skill already exists' });
  }

  const validSources = ['skills.sh', 'github'];
  const skillSource = validSources.includes(source) ? source : 'github';

  const validCategories = ['frontend', 'backend', 'devtools', 'security', 'ai-agents', 'mobile', 'content', 'design', 'data', 'general'];
  const skillCategory = validCategories.includes(category) ? category : null;

  db.prepare(`
    INSERT INTO skills (id, name, description, source, category)
    VALUES (?, ?, ?, ?, ?)
  `).run(id, name, description || null, skillSource, skillCategory);

  const skill = db.prepare('SELECT * FROM skills WHERE id = ?').get(id) as Skill;

  res.status(201).json(skill);
});

export default router;
