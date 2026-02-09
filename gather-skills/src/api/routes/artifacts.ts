import { Router } from 'express';
import { existsSync, readFileSync } from 'fs';
import { dirname, join, resolve, extname } from 'path';
import { getDb, getDbPath } from '../../db/index.js';

const router = Router();

// Mime types that can be served inline
const INLINE_MIME_TYPES = new Set([
  'text/html', 'text/css', 'text/plain', 'text/markdown',
  'application/javascript', 'application/json',
  'image/png', 'image/jpeg', 'image/gif', 'image/svg+xml', 'image/webp',
  'application/pdf',
]);

// GET /artifacts/:reviewId/*
router.get('/:reviewId/*', (req, res) => {
  const db = getDb();
  const reviewId = req.params.reviewId;
  const filePath = (req.params as Record<string, string>)[0]; // everything after reviewId/

  if (!filePath) {
    return res.status(400).json({ error: 'File path required' });
  }

  // Verify review exists
  const review = db.prepare('SELECT id FROM reviews WHERE id = ?').get(reviewId);
  if (!review) {
    return res.status(404).json({ error: 'Review not found' });
  }

  // Build absolute path and prevent traversal
  const dataDir = dirname(getDbPath());
  const artifactBase = resolve(join(dataDir, 'artifacts', reviewId));
  const requestedPath = resolve(join(artifactBase, filePath));

  if (!requestedPath.startsWith(artifactBase + '/')) {
    return res.status(403).json({ error: 'Access denied' });
  }

  if (!existsSync(requestedPath)) {
    return res.status(404).json({ error: 'Artifact not found' });
  }

  // Determine content type from extension
  const ext = extname(requestedPath).toLowerCase();
  const MIME_MAP: Record<string, string> = {
    '.html': 'text/html',
    '.css': 'text/css',
    '.js': 'application/javascript',
    '.json': 'application/json',
    '.png': 'image/png',
    '.jpg': 'image/jpeg',
    '.jpeg': 'image/jpeg',
    '.gif': 'image/gif',
    '.svg': 'image/svg+xml',
    '.webp': 'image/webp',
    '.pdf': 'application/pdf',
    '.md': 'text/markdown',
    '.txt': 'text/plain',
    '.xml': 'application/xml',
    '.yaml': 'application/yaml',
    '.yml': 'application/yaml',
    '.pptx': 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
    '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    '.xlsx': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    '.zip': 'application/zip',
    '.tar': 'application/x-tar',
    '.gz': 'application/gzip',
  };

  const mimeType = MIME_MAP[ext] || 'application/octet-stream';

  // Security headers
  res.setHeader('X-Content-Type-Options', 'nosniff');

  // HTML and SVG get sandboxed CSP — no allow-same-origin to prevent cookie/storage access
  if (mimeType === 'text/html' || mimeType === 'image/svg+xml') {
    res.setHeader('Content-Security-Policy', 'sandbox allow-scripts');
  }

  // Only serve whitelisted types inline; everything else is a download
  if (INLINE_MIME_TYPES.has(mimeType)) {
    res.setHeader('Content-Type', mimeType);
  } else {
    res.setHeader('Content-Type', mimeType);
    const fileName = filePath.split('/').pop() || 'download';
    res.setHeader('Content-Disposition', `attachment; filename="${fileName}"`);
  }

  const content = readFileSync(requestedPath);
  res.send(content);
});

export default router;
