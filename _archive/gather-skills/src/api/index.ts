import 'dotenv/config';
import express from 'express';
import { readFileSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { initDb } from '../db/index.js';
import skillsRouter from './routes/skills.js';
import reviewsRouter from './routes/reviews.js';
import proofsRouter from './routes/proofs.js';
import rankingsRouter from './routes/rankings.js';
import browseRouter from './routes/browse.js';
import authRouter from './routes/auth.js';
import artifactsRouter from './routes/artifacts.js';
import pagesRouter from './routes/pages.js';
import { optionalAuth } from './middleware/auth.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const app = express();
const PORT = parseInt(process.env.PORT || '3000', 10);

// View engine
app.set('view engine', 'ejs');
app.set('views', join(__dirname, '../views'));

// Static files
app.use(express.static(join(__dirname, '../../public')));

// Middleware
app.use(express.json({ limit: '75mb' }));

// Request logging
app.use((req, res, next) => {
  console.log(`${req.method} ${req.path}`);
  next();
});

// API Routes
app.use('/api/auth', authRouter);
app.use('/api/skills', optionalAuth, skillsRouter);
app.use('/api/reviews', optionalAuth, reviewsRouter);
app.use('/api/proofs', optionalAuth, proofsRouter);
app.use('/api/rankings', rankingsRouter);
app.use('/api/browse', browseRouter);

// Serve Gather SKILL.md as raw markdown
app.get('/api/skill', (req, res) => {
  try {
    const skillPath = join(__dirname, '../../skills/gather/SKILL.md');
    const content = readFileSync(skillPath, 'utf-8');
    res.type('text/markdown').send(content);
  } catch {
    res.status(500).json({ error: 'SKILL.md not found' });
  }
});

// Health check
app.get('/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// Artifact serving (before pages to avoid catch-all conflicts)
app.use('/artifacts', artifactsRouter);

// Web pages
app.use('/', pagesRouter);

// 404 handler
app.use((req: express.Request, res: express.Response) => {
  if (req.path.startsWith('/api/')) {
    res.status(404).json({ error: 'Not found' });
  } else {
    res.status(404).render('404', { title: 'Not Found' });
  }
});

// Error handler
app.use((err: Error, req: express.Request, res: express.Response, next: express.NextFunction) => {
  console.error('Error:', err.message);
  res.status(500).json({ error: 'Internal server error' });
});

// Initialize database and start server
initDb();
app.listen(PORT, () => {
  console.log(`Reskill API running on http://localhost:${PORT}`);
});

export default app;
