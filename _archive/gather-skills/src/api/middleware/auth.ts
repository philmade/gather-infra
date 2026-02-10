import { Request, Response, NextFunction } from 'express';
import { getDb } from '../../db/index.js';

export interface Agent {
  id: string;
  moltbook_id: string | null;
  name: string;
  description: string | null;
  avatar: string | null;
  twitter_handle: string | null;
  karma: number;
  api_key: string;
  review_count: number;
  created_at: string;
}

declare global {
  namespace Express {
    interface Request {
      agent?: Agent;
    }
  }
}

/**
 * Auth middleware - validates API key and attaches agent to request
 *
 * Usage:
 *   app.use('/api/reviews', authMiddleware, reviewsRouter);  // Required
 *   app.use('/api/skills', authMiddleware({ required: false }), skillsRouter);  // Optional
 */
export function authMiddleware(options: { required?: boolean } = {}) {
  const { required = true } = options;

  return (req: Request, res: Response, next: NextFunction) => {
    const apiKey = req.headers['x-api-key'] as string;

    if (!apiKey) {
      if (required) {
        return res.status(401).json({ error: 'API key required. Use X-Api-Key header.' });
      }
      return next();
    }

    const db = getDb();
    const agent = db.prepare('SELECT * FROM agents WHERE api_key = ?').get(apiKey) as Agent | undefined;

    if (!agent) {
      if (required) {
        return res.status(401).json({ error: 'Invalid API key' });
      }
      return next();
    }

    req.agent = agent;
    next();
  };
}

/**
 * Require auth - shorthand for required auth
 */
export const requireAuth = authMiddleware({ required: true });

/**
 * Optional auth - attaches agent if present but doesn't require it
 */
export const optionalAuth = authMiddleware({ required: false });
