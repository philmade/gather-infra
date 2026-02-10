import { Router } from 'express';
import { getDb } from '../../db/index.js';
import { verifyAttestation } from '../../lib/attestation.js';
import type { Proof } from '../../lib/types.js';

const router = Router();

// GET /api/proofs/:id - Get proof details
router.get('/:id', (req, res) => {
  const db = getDb();
  const proofId = req.params.id;

  const proof = db.prepare(`
    SELECT p.*, r.skill_id, r.task
    FROM proofs p
    JOIN reviews r ON p.review_id = r.id
    WHERE p.id = ?
  `).get(proofId) as (Proof & { skill_id: string; task: string }) | undefined;

  if (!proof) {
    return res.status(404).json({ error: 'Proof not found' });
  }

  // Parse JSON fields
  res.json({
    ...proof,
    claim_data: JSON.parse(proof.claim_data),
    signatures: JSON.parse(proof.signatures),
    witnesses: JSON.parse(proof.witnesses),
  });
});

// POST /api/proofs/:id/verify - Verify a proof
router.post('/:id/verify', async (req, res) => {
  const db = getDb();
  const proofId = req.params.id;

  const proof = db.prepare('SELECT * FROM proofs WHERE id = ?').get(proofId) as Proof | undefined;

  if (!proof) {
    return res.status(404).json({ error: 'Proof not found' });
  }

  // Verify the Ed25519 signature
  const witnesses = JSON.parse(proof.witnesses) as Array<{ type: string; public_key: string }>;
  const signatures = JSON.parse(proof.signatures) as string[];

  if (!witnesses.length || !signatures.length) {
    return res.json({ id: proofId, verified: false, message: 'No signatures found' });
  }

  const witness = witnesses[0];
  const signature = signatures[0];

  const isValid = verifyAttestation({
    id: proof.id,
    version: '1.0.0',
    execution_hash: proof.identifier,
    payload: JSON.parse(proof.claim_data),
    signature,
    public_key: witness.public_key,
  });

  db.prepare('UPDATE proofs SET verified = ? WHERE id = ?').run(isValid ? 1 : 0, proofId);

  res.json({
    id: proofId,
    verified: isValid,
    message: isValid ? 'Signature verified successfully' : 'Signature verification failed',
  });
});

// GET /api/proofs - List proofs
router.get('/', (req, res) => {
  const db = getDb();
  const limit = Math.min(100, parseInt(req.query.limit as string) || 20);
  const verified = req.query.verified;

  let query = `
    SELECT p.id, p.review_id, p.verified, p.created_at, r.skill_id
    FROM proofs p
    JOIN reviews r ON p.review_id = r.id
  `;

  const params: (string | number)[] = [];
  if (verified !== undefined) {
    query += ' WHERE p.verified = ?';
    params.push(verified === 'true' ? 1 : 0);
  }

  query += ' ORDER BY p.created_at DESC LIMIT ?';
  params.push(limit);

  const proofs = db.prepare(query).all(...params);

  res.json({ proofs });
});

export default router;
