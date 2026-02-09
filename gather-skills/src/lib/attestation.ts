/**
 * Simple cryptographic attestations for execution proofs
 *
 * MVP approach: Sign execution hashes with Ed25519
 * Can upgrade to ZK proofs (Reclaim, etc.) later
 */

import { createHash, generateKeyPairSync, sign, verify, randomUUID } from 'crypto';
import { existsSync, readFileSync, writeFileSync, mkdirSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const KEYS_PATH = join(__dirname, '../../data/keys.json');

interface KeyPair {
  publicKey: string;
  privateKey: string;
  createdAt: string;
}

interface ExecutionData {
  skill_id: string;
  task: string;
  cli_output: string;
  score: number | null;
  what_worked: string | null;
  what_failed: string | null;
  execution_time_ms: number | null;
}

export interface Attestation {
  id: string;
  version: string;
  execution_hash: string;
  payload: {
    skill_id: string;
    task_hash: string;
    output_hash: string;
    score: number | null;
    timestamp: number;
  };
  signature: string;
  public_key: string;
}

function getOrCreateKeyPair(): KeyPair {
  if (existsSync(KEYS_PATH)) {
    const data = readFileSync(KEYS_PATH, 'utf-8');
    return JSON.parse(data) as KeyPair;
  }

  // Generate new Ed25519 keypair
  const { publicKey, privateKey } = generateKeyPairSync('ed25519', {
    publicKeyEncoding: { type: 'spki', format: 'pem' },
    privateKeyEncoding: { type: 'pkcs8', format: 'pem' },
  });

  const keyPair: KeyPair = {
    publicKey,
    privateKey,
    createdAt: new Date().toISOString(),
  };

  // Ensure directory exists
  const dir = dirname(KEYS_PATH);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  writeFileSync(KEYS_PATH, JSON.stringify(keyPair, null, 2));
  console.log('Generated new attestation keypair');

  return keyPair;
}

export function hashContent(content: string): string {
  return createHash('sha256').update(content).digest('hex');
}

export function createAttestation(data: ExecutionData): Attestation {
  const keyPair = getOrCreateKeyPair();
  const timestamp = Date.now();

  // Create payload with hashed sensitive data
  const payload = {
    skill_id: data.skill_id,
    task_hash: hashContent(data.task),
    output_hash: hashContent(data.cli_output || ''),
    score: data.score,
    timestamp,
  };

  // Create execution hash (hash of all relevant data)
  const executionHash = hashContent(JSON.stringify({
    ...payload,
    what_worked: data.what_worked,
    what_failed: data.what_failed,
    execution_time_ms: data.execution_time_ms,
  }));

  // Sign the execution hash
  const signature = sign(
    null,
    Buffer.from(executionHash),
    keyPair.privateKey
  ).toString('base64');

  return {
    id: randomUUID(),
    version: '1.0.0',
    execution_hash: executionHash,
    payload,
    signature,
    public_key: keyPair.publicKey,
  };
}

export function verifyAttestation(attestation: Attestation): boolean {
  try {
    const isValid = verify(
      null,
      Buffer.from(attestation.execution_hash),
      attestation.public_key,
      Buffer.from(attestation.signature, 'base64')
    );
    return isValid;
  } catch {
    return false;
  }
}

export function getPublicKey(): string {
  const keyPair = getOrCreateKeyPair();
  return keyPair.publicKey;
}
