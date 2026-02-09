import { Router } from 'express';
import { readFileSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { getDb } from '../../db/index.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const router = Router();

// Helper to render with layout
function render(res: any, view: string, data: Record<string, any>) {
  res.render(view, {
    ...data,
    body: '', // Will be replaced by EJS include
  });
}

// Home - Top skills
router.get('/', (req, res) => {
  const db = getDb();

  // Reviewed skills (have at least 1 review with a score, exclude test/mock skills)
  const reviewedSkills = db.prepare(`
    SELECT
      s.*,
      (SELECT COUNT(*) FROM proofs p JOIN reviews r ON p.review_id = r.id WHERE r.skill_id = s.id AND p.verified = 1) as verified_proofs
    FROM skills s
    WHERE s.review_count > 0 AND s.avg_score IS NOT NULL
      AND s.id NOT LIKE 'test/%' AND s.id NOT LIKE 'gather/%'
    ORDER BY s.rank_score DESC NULLS LAST, s.avg_score DESC
    LIMIT 20
  `).all();

  // Popular by installs (exclude already-shown reviewed skills)
  const reviewedIds = (reviewedSkills as any[]).map((s: any) => s.id);
  const placeholders = reviewedIds.length > 0 ? reviewedIds.map(() => '?').join(',') : "'__none__'";
  const popularSkills = db.prepare(`
    SELECT s.* FROM skills s
    WHERE s.id NOT IN (${placeholders})
    ORDER BY s.installs DESC NULLS LAST
    LIMIT 15
  `).all(...reviewedIds);

  const stats = {
    skills: (db.prepare('SELECT COUNT(*) as c FROM skills').get() as any).c,
    categories: (db.prepare('SELECT COUNT(DISTINCT category) as c FROM skills WHERE category IS NOT NULL').get() as any).c,
  };

  res.render('home', { title: 'Home', reviewedSkills, popularSkills, stats });
});

// Skill detail
router.get('/skills/:id(*)', (req, res) => {
  const db = getDb();
  const skillId = req.params.id;

  const skill = db.prepare('SELECT * FROM skills WHERE id = ?').get(skillId);

  if (!skill) {
    return res.status(404).send('Skill not found');
  }

  const reviews = db.prepare(`
    SELECT r.*, a.name as agent_name, a.id as agent_id, a.twitter_handle as agent_twitter
    FROM reviews r
    LEFT JOIN agents a ON r.agent_id = a.id
    WHERE r.skill_id = ? AND r.status = 'complete'
    ORDER BY r.created_at DESC
    LIMIT 20
  `).all(skillId);

  // Get artifact metadata and proof data for each review
  for (const review of reviews as any[]) {
    const artifacts = db.prepare(
      'SELECT id, file_name, mime_type, size_bytes FROM artifacts WHERE review_id = ?'
    ).all(review.id) as Array<{ id: string; file_name: string; mime_type: string; size_bytes: number }>;
    review.artifact_count = artifacts.length;
    review.artifacts = artifacts;

    // Detect web builds (has an index.html)
    const indexHtml = artifacts.find(a => a.file_name === 'index.html' || a.file_name.endsWith('/index.html'));
    review.has_web_build = !!indexHtml;
    review.web_build_path = indexHtml ? `/artifacts/${review.id}/${indexHtml.file_name}` : null;

    // Get proof details if exists
    if (review.proof_id) {
      const proof = db.prepare(
        'SELECT id, identifier, signatures, verified, created_at FROM proofs WHERE id = ?'
      ).get(review.proof_id) as any;
      if (proof) {
        review.proof = {
          id: proof.id,
          verified: proof.verified,
          signature_hash: proof.identifier?.substring(0, 16) || proof.id.substring(0, 16),
          created_at: proof.created_at,
        };
      }
    }
  }

  const proofCount = (db.prepare(`
    SELECT COUNT(*) as c FROM proofs p
    JOIN reviews r ON p.review_id = r.id
    WHERE r.skill_id = ? AND p.verified = 1
  `).get(skillId) as any).c;

  res.render('skill', { title: (skill as any).name, skill, reviews, proofCount });
});

// Leaderboard
router.get('/leaderboard', (req, res) => {
  const db = getDb();

  const agents = db.prepare(`
    SELECT a.*,
      (SELECT COUNT(*) FROM reviews r WHERE r.agent_id = a.id AND r.security_score IS NOT NULL AND r.status = 'complete') as security_reviews
    FROM agents a
    ORDER BY a.review_count DESC, a.karma DESC
    LIMIT 50
  `).all();

  res.render('leaderboard', { title: 'Leaderboard', agents });
});

// Agent profile
router.get('/agents/:id', (req, res) => {
  const db = getDb();
  const agentId = req.params.id;

  const agent = db.prepare('SELECT * FROM agents WHERE id = ?').get(agentId);

  if (!agent) {
    return res.status(404).send('Agent not found');
  }

  const reviews = db.prepare(`
    SELECT r.*, s.name as skill_name
    FROM reviews r
    JOIN skills s ON r.skill_id = s.id
    WHERE r.agent_id = ? AND r.status = 'complete'
    ORDER BY r.created_at DESC
    LIMIT 20
  `).all(agentId);

  // Calculate rank
  const allAgents = db.prepare('SELECT id FROM agents ORDER BY review_count DESC, karma DESC').all() as any[];
  const rank = allAgents.findIndex((a: any) => a.id === agentId) + 1;

  res.render('agent', { title: (agent as any).name, agent, reviews, rank });
});

// Gather SKILL.md rendered as a web page
router.get('/skill', (req, res) => {
  try {
    const skillPath = join(__dirname, '../../../skills/gather/SKILL.md');
    const content = readFileSync(skillPath, 'utf-8');
    res.render('skill-doc', { title: 'Gather Skill', content });
  } catch {
    res.status(500).send('SKILL.md not found');
  }
});

// About
router.get('/about', (req, res) => {
  res.render('about', { title: 'About' });
});

// Search
router.get('/search', (req, res) => {
  const db = getDb();
  const q = (req.query.q as string || '').trim();
  const category = (req.query.category as string || '').trim();
  const sort = (req.query.sort as string || 'rank');
  const minSecurity = (req.query.min_security as string || '').trim();

  // Get distinct categories for dropdown
  const categories = db.prepare(`
    SELECT DISTINCT category FROM skills
    WHERE category IS NOT NULL
    ORDER BY category
  `).all().map((r: any) => r.category) as string[];

  // Build query
  const SORT_MAP: Record<string, string> = {
    rank: 'rank_score DESC NULLS LAST, review_count DESC',
    installs: 'installs DESC NULLS LAST',
    reviews: 'review_count DESC',
    security: 'avg_security_score DESC NULLS LAST',
    newest: 'created_at DESC',
  };

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

  if (minSecurity) {
    const minSecVal = parseFloat(minSecurity);
    if (!isNaN(minSecVal)) {
      conditions.push('avg_security_score >= ?');
      params.push(minSecVal);
    }
  }

  const whereClause = conditions.length > 0 ? `WHERE ${conditions.join(' AND ')}` : '';
  const orderBy = SORT_MAP[sort] || SORT_MAP.rank;

  const skills = db.prepare(`
    SELECT * FROM skills
    ${whereClause}
    ORDER BY ${orderBy}, created_at DESC
    LIMIT 100
  `).all(...params);

  const total = (db.prepare(`SELECT COUNT(*) as count FROM skills ${whereClause}`).get(...params) as { count: number }).count;

  res.render('search', {
    title: 'Search',
    skills,
    total,
    q,
    category,
    sort,
    minSecurity,
    categories,
  });
});

// API Docs
router.get('/docs', (req, res) => {
  const baseUrl = process.env.API_URL || `http://localhost:${process.env.PORT || 3000}`;
  res.render('docs', { title: 'API Docs', baseUrl });
});

// Category descriptions for llms.txt
const CATEGORY_DESCRIPTIONS: Record<string, string> = {
  frontend: 'React, Vue, Tailwind, UI components',
  backend: 'APIs, databases, auth',
  devtools: 'Testing, debugging, CI/CD, code review',
  security: 'Analysis, auditing, vulnerability detection',
  'ai-agents': 'MCP servers, agent tools, LLM integration',
  mobile: 'React Native, Expo, native UI',
  content: 'Copywriting, marketing, SEO, documents',
  design: 'Canvas, art, animation, themes',
  data: 'Analytics, scraping, monitoring',
  general: 'Uncategorized skills',
};

// GET /llms.txt — AI-navigable site index
router.get('/llms.txt', (req, res) => {
  const db = getDb();

  const categories = db.prepare(`
    SELECT category, COUNT(*) as count
    FROM skills
    WHERE category IS NOT NULL
    GROUP BY category
    ORDER BY count DESC
  `).all() as Array<{ category: string; count: number }>;

  const totalSkills = (db.prepare('SELECT COUNT(*) as c FROM skills').get() as { c: number }).c;

  const topSkills = db.prepare(`
    SELECT id, name, installs FROM skills
    ORDER BY installs DESC NULLS LAST
    LIMIT 10
  `).all() as Array<{ id: string; name: string; installs: number }>;

  let text = `# Gather Skills\n\n`;
  text += `> AI skill review platform. ${totalSkills} skills across ${categories.length} categories, reviewed by agents with cryptographic proofs.\n\n`;

  text += `## Categories\n`;
  for (const cat of categories) {
    const desc = CATEGORY_DESCRIPTIONS[cat.category] || '';
    text += `- [${cat.category}](/c/${cat.category}): ${cat.count} skills — ${desc}\n`;
  }

  text += `\n## Top Skills\n`;
  for (const skill of topSkills) {
    const installs = skill.installs ? `${skill.installs.toLocaleString()} installs` : 'no install data';
    text += `- [${skill.id}](/s/${skill.id}): ${skill.name} (${installs})\n`;
  }

  text += `\n## API\n`;
  text += `- [API Documentation](/docs)\n`;
  text += `- [Gather SKILL.md](/api/skill): Install this skill to interact with the platform\n`;

  res.type('text/plain').send(text);
});

// GET /c/:category — Category listing
router.get('/c/:category', (req, res) => {
  const db = getDb();
  const category = req.params.category;

  const skills = db.prepare(`
    SELECT id, name, installs, review_count, avg_score FROM skills
    WHERE category = ?
    ORDER BY installs DESC NULLS LAST, review_count DESC
  `).all(category) as Array<{ id: string; name: string; installs: number; review_count: number; avg_score: number | null }>;

  if (skills.length === 0) {
    return res.status(404).type('text/plain').send(`# Not Found\n\nNo skills found in category "${category}".`);
  }

  const desc = CATEGORY_DESCRIPTIONS[category] || '';
  let text = `# ${category.charAt(0).toUpperCase() + category.slice(1)} Skills\n\n`;
  text += `${skills.length} skills${desc ? ` for ${desc.toLowerCase()}` : ''}.\n\n`;

  for (const skill of skills) {
    const installs = skill.installs ? `${skill.installs.toLocaleString()} installs` : '0 installs';
    const reviews = skill.review_count > 0 ? `, ${skill.review_count} reviews` : '';
    const score = skill.avg_score !== null ? `, avg ${skill.avg_score.toFixed(1)}/10` : '';
    text += `- [${skill.id}](/s/${skill.id}): ${installs}${reviews}${score}\n`;
  }

  res.type('text/plain').send(text);
});

// GET /s/:id(*) — Skill detail (text/plain)
router.get('/s/:id(*)', (req, res) => {
  const db = getDb();
  const skillId = req.params.id;

  const skill = db.prepare('SELECT * FROM skills WHERE id = ?').get(skillId) as any;

  if (!skill) {
    return res.status(404).type('text/plain').send(`# Not Found\n\nSkill "${skillId}" not found.`);
  }

  const reviews = db.prepare(`
    SELECT score, what_worked, what_failed, skill_feedback, security_score, security_notes, agent_model, created_at
    FROM reviews
    WHERE skill_id = ? AND status = 'complete'
    ORDER BY created_at DESC
    LIMIT 10
  `).all(skillId) as Array<{ score: number; what_worked: string; what_failed: string; skill_feedback: string; security_score: number | null; security_notes: string | null; agent_model: string; created_at: string }>;

  // Extract author and source from the ID
  const parts = skill.id.split('/');
  const author = parts[0] || 'unknown';
  const source = parts.length >= 3 ? `${parts[0]}/${parts[1]}` : skill.id;

  let text = `# ${skill.name || skill.id}\n\n`;
  text += `Author: ${author}\n`;
  text += `Source: ${source}\n`;
  if (skill.category) text += `Category: ${skill.category}\n`;
  text += `Installs: ${skill.installs ? skill.installs.toLocaleString() : '0'}\n`;
  text += `Reviews: ${skill.review_count}\n`;
  text += `Average Score: ${skill.avg_score !== null ? `${skill.avg_score.toFixed(1)}/10` : 'n/a'}\n`;

  if (skill.description) {
    text += `\n## Description\n\n${skill.description}\n`;
  }

  text += `\n## Reviews\n\n`;
  if (reviews.length === 0) {
    text += `No reviews yet.\n`;
  } else {
    for (const review of reviews) {
      text += `### Score: ${review.score}/10`;
      if (review.security_score != null) text += ` | Security: ${review.security_score}/10`;
      if (review.agent_model) text += ` (${review.agent_model})`;
      text += `\n`;
      if (review.what_worked) text += `- Worked: ${review.what_worked}\n`;
      if (review.what_failed) text += `- Failed: ${review.what_failed}\n`;
      if (review.security_notes) text += `- Security: ${review.security_notes}\n`;
      if (review.skill_feedback) text += `- Feedback: ${review.skill_feedback}\n`;
      text += `\n`;
    }
  }

  text += `## Links\n\n`;
  text += `- Install: npx @gathers/skills add ${skill.id}\n`;
  text += `- Review: POST /api/reviews/submit { "skill_id": "${skill.id}", "task": "..." }\n`;
  text += `- Details: /api/skills/${skill.id}\n`;

  res.type('text/plain').send(text);
});

export default router;
