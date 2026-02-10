import { Command } from 'commander';
import chalk from 'chalk';
import { getDb } from '../../db/index.js';
import { verifyAttestation } from '../../lib/attestation.js';

const command = new Command('proof')
  .description('Get or verify proof for a review')
  .argument('<review_id>', 'Review ID to get proof for')
  .option('--verify', 'Verify the proof', false)
  .option('--json', 'Output as JSON', false)
  .action((reviewId: string, options: { verify: boolean; json: boolean }) => {
    const db = getDb();

    // Get review
    const review = db.prepare(`
      SELECT r.*, s.name as skill_name
      FROM reviews r
      JOIN skills s ON r.skill_id = s.id
      WHERE r.id = ?
    `).get(reviewId) as {
      id: string;
      skill_id: string;
      skill_name: string;
      status: string;
      proof_id: string | null;
    } | undefined;

    if (!review) {
      console.log(chalk.red('Review not found'));
      process.exit(1);
    }

    if (review.status !== 'complete') {
      console.log(chalk.yellow(`Review is ${review.status}, proof not available`));
      process.exit(1);
    }

    if (!review.proof_id) {
      console.log(chalk.yellow('No proof generated for this review'));
      console.log(chalk.dim('Proof generation will be available in Phase 2'));
      process.exit(0);
    }

    // Get proof
    const proof = db.prepare('SELECT * FROM proofs WHERE id = ?').get(review.proof_id) as {
      id: string;
      review_id: string;
      claim_data: string;
      identifier: string;
      signatures: string;
      witnesses: string;
      verified: boolean;
      created_at: string;
    } | undefined;

    if (!proof) {
      console.log(chalk.red('Proof not found'));
      process.exit(1);
    }

    if (options.json) {
      console.log(JSON.stringify({
        ...proof,
        claim_data: JSON.parse(proof.claim_data),
        signatures: JSON.parse(proof.signatures),
        witnesses: JSON.parse(proof.witnesses),
      }, null, 2));
      return;
    }

    console.log(chalk.bold('Proof Details'));
    console.log();
    console.log(chalk.dim('Proof ID:'), proof.id);
    console.log(chalk.dim('Review ID:'), proof.review_id);
    console.log(chalk.dim('Skill:'), review.skill_name);
    console.log(chalk.dim('Verified:'), proof.verified ? chalk.green('Yes') : chalk.yellow('No'));
    console.log(chalk.dim('Created:'), proof.created_at);
    console.log();
    console.log(chalk.dim('Identifier:'), proof.identifier);
    console.log();
    console.log(chalk.dim('Claim Data:'));
    console.log(proof.claim_data);

    if (options.verify) {
      console.log();
      console.log(chalk.blue('Verifying signature...'));

      const witnesses = JSON.parse(proof.witnesses) as Array<{ type: string; public_key: string }>;
      const signatures = JSON.parse(proof.signatures) as string[];

      if (!witnesses.length || !signatures.length) {
        console.log(chalk.red('✗ No signatures found'));
        return;
      }

      const isValid = verifyAttestation({
        id: proof.id,
        version: '1.0.0',
        execution_hash: proof.identifier,
        payload: JSON.parse(proof.claim_data),
        signature: signatures[0],
        public_key: witnesses[0].public_key,
      });

      db.prepare('UPDATE proofs SET verified = ? WHERE id = ?').run(isValid ? 1 : 0, proof.id);

      if (isValid) {
        console.log(chalk.green('✓ Signature verified (Ed25519)'));
      } else {
        console.log(chalk.red('✗ Signature verification failed'));
      }
    }
  });

export default command;
