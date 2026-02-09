import { Router } from 'express';
import { nanoid } from 'nanoid';
import { mkdirSync, writeFileSync } from 'fs';
import { dirname, join, normalize, resolve } from 'path';
import { getDb, getDbPath } from '../../db/index.js';
import { executeReview } from '../../worker/executor.js';
import { updateSkillRanking } from '../../lib/ranking.js';
import { createAttestation } from '../../lib/attestation.js';
import type { Review } from '../../lib/types.js';

const router = Router();

// POST /api/reviews - Create a new review (async)
router.post('/', async (req, res) => {
  const db = getDb();
  const { skill_id, task } = req.body;

  if (!skill_id || !task) {
    return res.status(400).json({ error: 'skill_id and task are required' });
  }

  // Check if skill exists, if not create it
  let skill = db.prepare('SELECT id FROM skills WHERE id = ?').get(skill_id);
  if (!skill) {
    // Auto-create skill from ID
    const name = skill_id.split('/').pop() || skill_id;
    db.prepare(`
      INSERT INTO skills (id, name, source)
      VALUES (?, ?, 'github')
    `).run(skill_id, name);
  }

  // Create review (link to agent if authenticated)
  const reviewId = nanoid();
  const agentId = req.agent?.id || null;

  db.prepare(`
    INSERT INTO reviews (id, skill_id, agent_id, task, status)
    VALUES (?, ?, ?, ?, 'pending')
  `).run(reviewId, skill_id, agentId, task);

  const review = db.prepare('SELECT * FROM reviews WHERE id = ?').get(reviewId) as Review;

  // Start execution in background
  executeReview(reviewId, skill_id, task).catch((err) => {
    console.error(`Review ${reviewId} failed:`, err);
  });

  res.status(202).json({
    id: review.id,
    status: review.status,
    message: 'Review started',
  });
});

/**
 * POST /api/reviews/submit - Submit a completed review from CLI
 *
 * This endpoint accepts reviews that were executed locally by the CLI
 * and stores them in the remote database.
 */
router.post('/submit', (req, res) => {
  const db = getDb();
  const {
    skill_id,
    task,
    score,
    what_worked,
    what_failed,
    skill_feedback,
    security_score,
    security_notes,
    runner_type,
    permission_mode,
    execution_time_ms,
    cli_output,
    proof,
  } = req.body;

  // Require authentication first
  if (!req.agent) {
    return res.status(401).json({ error: 'API key required to submit reviews' });
  }

  if (!skill_id || !task) {
    return res.status(400).json({ error: 'skill_id and task are required' });
  }

  if (!score || typeof score !== 'number' || score < 1 || score > 10) {
    return res.status(400).json({ error: 'score is required (number 1-10)' });
  }

  // Check if skill exists, if not create it
  let skill = db.prepare('SELECT id FROM skills WHERE id = ?').get(skill_id);
  if (!skill) {
    const name = skill_id.split('/').pop() || skill_id;
    db.prepare(`
      INSERT INTO skills (id, name, source)
      VALUES (?, ?, 'github')
    `).run(skill_id, name);
  }

  // Create the review with all the data
  const reviewId = nanoid();

  db.prepare(`
    INSERT INTO reviews (
      id, skill_id, agent_id, task, status, score,
      what_worked, what_failed, skill_feedback,
      security_score, security_notes,
      runner_type, permission_mode, agent_model,
      execution_time_ms, cli_output
    ) VALUES (?, ?, ?, ?, 'complete', ?, ?, ?, ?, ?, ?, ?, ?, 'claude-sonnet', ?, ?)
  `).run(
    reviewId,
    skill_id,
    req.agent.id,
    task,
    score || null,
    what_worked || null,
    what_failed || null,
    skill_feedback || null,
    security_score != null ? security_score : null,
    security_notes || null,
    runner_type || 'claude',
    permission_mode || 'default',
    execution_time_ms || null,
    cli_output || null
  );

  // Use client-side proof if provided (from the executor), otherwise generate server-side
  let proofId: string;
  if (proof && proof.id && proof.signature && proof.execution_hash && proof.public_key) {
    // Client ran the executor and generated a real proof tied to execution
    proofId = proof.id;
    db.prepare(`
      INSERT INTO proofs (id, review_id, claim_data, identifier, signatures, witnesses, verified)
      VALUES (?, ?, ?, ?, ?, ?, 1)
    `).run(
      proof.id,
      reviewId,
      JSON.stringify(proof.payload || { submitted_by: req.agent.id, skill_id, task }),
      proof.execution_hash,
      JSON.stringify([proof.signature]),
      JSON.stringify([{ type: 'ed25519', public_key: proof.public_key }])
    );
  } else {
    // No client proof — generate a server-side attestation
    const attestation = createAttestation({
      skill_id,
      task,
      cli_output: cli_output || '',
      score: score || null,
      what_worked: what_worked || null,
      what_failed: what_failed || null,
      execution_time_ms: execution_time_ms || null,
    });
    proofId = attestation.id;
    db.prepare(`
      INSERT INTO proofs (id, review_id, claim_data, identifier, signatures, witnesses, verified)
      VALUES (?, ?, ?, ?, ?, ?, 1)
    `).run(
      attestation.id,
      reviewId,
      JSON.stringify(attestation.payload),
      attestation.execution_hash,
      JSON.stringify([attestation.signature]),
      JSON.stringify([{ type: 'ed25519', public_key: attestation.public_key }])
    );
  }

  // Link proof to review
  db.prepare('UPDATE reviews SET proof_id = ? WHERE id = ?').run(proofId, reviewId);

  // Store artifacts from client
  let artifactCount = 0;
  const artifacts = req.body.artifacts;
  if (Array.isArray(artifacts) && artifacts.length > 0) {
    const dataDir = dirname(getDbPath());
    const artifactDir = join(dataDir, 'artifacts', reviewId);
    let totalBytes = 0;
    const maxTotal = 50 * 1024 * 1024;  // 50MB
    const maxPerFile = 10 * 1024 * 1024; // 10MB
    const maxFiles = 200;

    for (const art of artifacts) {
      if (artifactCount >= maxFiles) break;
      if (!art.file_name || !art.content_base64) continue;

      // Sanitize file_name: strip leading slashes, reject path traversal
      const sanitized = normalize(art.file_name).replace(/^(\.\.(\/|\\|$))+/, '').replace(/^\/+/, '');
      if (!sanitized || sanitized.startsWith('..') || resolve(artifactDir, sanitized).indexOf(resolve(artifactDir)) !== 0) {
        continue;
      }

      let buf: Buffer;
      try {
        buf = Buffer.from(art.content_base64, 'base64');
      } catch {
        continue;
      }

      if (buf.length > maxPerFile) continue;
      if (totalBytes + buf.length > maxTotal) break;

      const destPath = join(artifactDir, sanitized);
      mkdirSync(dirname(destPath), { recursive: true });
      writeFileSync(destPath, buf);

      totalBytes += buf.length;
      artifactCount++;

      const artifactId = nanoid();
      const filePath = `artifacts/${reviewId}/${sanitized}`;
      db.prepare(`
        INSERT INTO artifacts (id, review_id, file_name, mime_type, file_path, size_bytes)
        VALUES (?, ?, ?, ?, ?, ?)
      `).run(artifactId, reviewId, sanitized, art.mime_type || 'application/octet-stream', filePath, buf.length);
    }
  }

  // Update skill stats
  const stats = db.prepare(`
    SELECT COUNT(*) as count, AVG(score) as avg
    FROM reviews
    WHERE skill_id = ? AND status = 'complete' AND score IS NOT NULL
  `).get(skill_id) as { count: number; avg: number | null };

  const secStats = db.prepare(`
    SELECT AVG(security_score) as avg
    FROM reviews
    WHERE skill_id = ? AND status = 'complete' AND security_score IS NOT NULL
  `).get(skill_id) as { avg: number | null };

  db.prepare(`
    UPDATE skills
    SET review_count = ?, avg_score = ?, avg_security_score = ?
    WHERE id = ?
  `).run(stats.count, stats.avg, secStats.avg, skill_id);

  // Update ranking
  updateSkillRanking(db, skill_id);

  // Update agent review count
  db.prepare(`
    UPDATE agents SET review_count = review_count + 1 WHERE id = ?
  `).run(req.agent.id);

  res.status(201).json({
    message: 'Review submitted successfully',
    review_id: reviewId,
    skill_id,
    score,
    proof_id: proofId,
    artifact_count: artifactCount,
  });
});

// GET /api/reviews/:id - Get review status and results
router.get('/:id', (req, res) => {
  const db = getDb();
  const reviewId = req.params.id;

  const review = db.prepare(`
    SELECT r.*, s.name as skill_name
    FROM reviews r
    JOIN skills s ON r.skill_id = s.id
    WHERE r.id = ?
  `).get(reviewId) as (Review & { skill_name: string }) | undefined;

  if (!review) {
    return res.status(404).json({ error: 'Review not found' });
  }

  // Get artifacts if complete
  let artifacts: Array<{ id: string; file_name: string; mime_type: string }> = [];
  if (review.status === 'complete') {
    artifacts = db.prepare(`
      SELECT id, file_name, mime_type
      FROM artifacts
      WHERE review_id = ?
    `).all(reviewId) as Array<{ id: string; file_name: string; mime_type: string }>;
  }

  // Get proof if exists
  let proof = null;
  if (review.proof_id) {
    proof = db.prepare('SELECT id, verified, created_at FROM proofs WHERE id = ?').get(review.proof_id);
  }

  res.json({
    ...review,
    artifacts,
    proof,
  });
});

// GET /api/reviews - List recent reviews
router.get('/', (req, res) => {
  const db = getDb();
  const limit = Math.max(1, Math.min(100, parseInt(req.query.limit as string) || 20));
  const status = req.query.status as string;

  let query = `
    SELECT r.id, r.skill_id, r.task, r.status, r.score, r.created_at, s.name as skill_name
    FROM reviews r
    JOIN skills s ON r.skill_id = s.id
  `;

  const params: (string | number)[] = [];
  if (status) {
    query += ' WHERE r.status = ?';
    params.push(status);
  }

  query += ' ORDER BY r.created_at DESC LIMIT ?';
  params.push(limit);

  const reviews = db.prepare(query).all(...params);

  res.json({ reviews });
});

export default router;
