/**
 * @gather/auth — Gather agent authentication for TypeScript services
 *
 * Validates JWTs issued by the Go auth service.
 * Used by gather-skills (Node/Express) to authenticate agents.
 *
 * Phase 1 will implement:
 * - validateJWT(token, signingKey) → AgentClaims
 * - verifySignature(publicKeyPEM, message, signatureBase64) → boolean
 * - parsePublicKeyPEM(pem) → KeyObject
 */

import { verify as cryptoVerify, createPublicKey, type KeyObject } from 'crypto';
import jwt from 'jsonwebtoken';

export interface AgentClaims {
  agent_id: string;
  pubkey_fp: string;
  iss: string;
  sub: string;
  iat: number;
  exp: number;
}

/**
 * Validate a Gather agent JWT and return the claims.
 */
export function validateJWT(token: string, signingKey: string): AgentClaims {
  return jwt.verify(token, signingKey) as AgentClaims;
}

/**
 * Parse a PEM-encoded Ed25519 public key.
 */
export function parsePublicKeyPEM(pem: string): KeyObject {
  return createPublicKey(pem);
}

/**
 * Verify an Ed25519 signature against a public key and message.
 * Ed25519 uses crypto.verify (not createVerify) — no digest algorithm needed.
 */
export function verifySignature(
  publicKeyPEM: string,
  message: Buffer,
  signatureBase64: string,
): boolean {
  const key = parsePublicKeyPEM(publicKeyPEM);
  return cryptoVerify(null, message, key, Buffer.from(signatureBase64, 'base64'));
}
