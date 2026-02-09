import { Router } from 'express';
import { getDb } from '../../db/index.js';

const router = Router();

/**
 * Subcategory taxonomy — keyword-based assignment from skill name/description/id.
 * Each category has subcategories with match keywords.
 * A skill matches the first subcategory whose keywords appear in its name, description, or id.
 */
const TAXONOMY: Record<string, { label: string; description: string; subcategories: Record<string, { label: string; keywords: string[] }> }> = {
  'ai-agents': {
    label: 'AI Agents',
    description: 'MCP servers, agent tools, browser automation, and LLM integration',
    subcategories: {
      'mcp': { label: 'MCP Servers', keywords: ['mcp', 'model context protocol', 'mcp-'] },
      'browser': { label: 'Browser Automation', keywords: ['browser', 'scraping', 'web-agent', 'puppeteer', 'playwright'] },
      'prompting': { label: 'Prompt Engineering', keywords: ['prompt', 'system-prompt', 'chain-of-thought'] },
      'patterns': { label: 'Architecture Patterns', keywords: ['pattern', 'architecture', 'composition', 'best-practice'] },
      'language': { label: 'Language-Specific', keywords: ['python', 'typescript', 'nodejs', 'rust', 'golang', 'java', 'sql'] },
      'tools': { label: 'Agent Tools', keywords: ['tool', 'skill-creator', 'find-skills', 'agent'] },
    },
  },
  'frontend': {
    label: 'Frontend',
    description: 'React, Vue, CSS, Tailwind, UI components, and web interfaces',
    subcategories: {
      'react': { label: 'React & Next.js', keywords: ['react', 'next', 'nextjs', 'vercel', 'jsx', 'tsx'] },
      'vue': { label: 'Vue.js', keywords: ['vue', 'nuxt', 'vuejs'] },
      'css': { label: 'CSS & Styling', keywords: ['css', 'tailwind', 'style', 'theme', 'design-system', 'brand'] },
      'animation': { label: 'Animation & Video', keywords: ['animation', 'remotion', 'motion', 'video', 'canvas'] },
      'native': { label: 'React Native & Mobile UI', keywords: ['native', 'expo', 'mobile', 'ios', 'android'] },
      'components': { label: 'UI Components', keywords: ['component', 'ui', 'ux', 'widget', 'artifact', 'interface', 'web-design'] },
      'testing': { label: 'Web Testing', keywords: ['test', 'accessibility', 'wcag', 'audit', 'performance', 'lighthouse'] },
    },
  },
  'devtools': {
    label: 'Developer Tools',
    description: 'Testing, debugging, CI/CD, code review, and workflow automation',
    subcategories: {
      'testing': { label: 'Testing', keywords: ['test', 'tdd', 'e2e', 'unit', 'integration', 'spec'] },
      'debugging': { label: 'Debugging', keywords: ['debug', 'profil', 'troubleshoot', 'diagnos'] },
      'git': { label: 'Git & Version Control', keywords: ['git', 'worktree', 'branch', 'commit', 'merge'] },
      'planning': { label: 'Planning & Architecture', keywords: ['plan', 'spec', 'architect', 'design'] },
      'code-review': { label: 'Code Review', keywords: ['review', 'code-review', 'quality', 'lint'] },
      'ci-cd': { label: 'CI/CD & Deployment', keywords: ['ci', 'cd', 'deploy', 'pipeline', 'workflow'] },
      'agents': { label: 'Agent Workflows', keywords: ['agent', 'subagent', 'parallel', 'dispatch'] },
    },
  },
  'content': {
    label: 'Content Creation',
    description: 'Documents, marketing, copywriting, and media generation',
    subcategories: {
      'marketing': { label: 'Marketing & SEO', keywords: ['marketing', 'seo', 'copywriting', 'copy-editing', 'social-content', 'ads', 'landing', 'cro', 'competitor', 'referral', 'popup', 'signup', 'paywall', 'onboarding', 'ab-test', 'form-cro', 'page-cro', 'email-sequence', 'paid-ads', 'schema-markup', 'free-tool', 'content-strategy'] },
      'strategy': { label: 'Strategy & Ideation', keywords: ['brainstorm', 'strategy', 'pricing', 'launch', 'ideation', 'analytics'] },
      'documents': { label: 'Document Generation', keywords: ['pdf', 'docx', 'pptx', 'xlsx', 'powerpoint', 'word', 'spreadsheet', 'slide', 'deck', 'illustrat'] },
    },
  },
  'security': {
    label: 'Security',
    description: 'Code auditing, vulnerability detection, threat modeling, and compliance',
    subcategories: {
      'blockchain': { label: 'Blockchain Security', keywords: ['solidity', 'smart-contract', 'blockchain', 'web3', 'solana', 'token-integration'] },
      'analysis': { label: 'Threat Analysis', keywords: ['threat', 'attack', 'stride', 'binary-analysis', 'forensic', 'coverage-analysis'] },
      'audit': { label: 'Audit & Compliance', keywords: ['audit', 'compliance', 'semgrep', 'wcag', 'owasp', 'seo-audit'] },
      'code-review': { label: 'Security Review', keywords: ['vulnerability', 'differential', 'sharp'] },
    },
  },
  'backend': {
    label: 'Backend',
    description: 'APIs, databases, authentication, and server-side development',
    subcategories: {
      'database': { label: 'Database', keywords: ['postgres', 'sqlite', 'database', 'migration', 'schema', 'sql', 'supabase'] },
      'auth': { label: 'Authentication', keywords: ['auth', 'oauth', 'jwt', 'session', 'login'] },
      'api': { label: 'API Design', keywords: ['api', 'openapi', 'rest', 'graphql', 'endpoint'] },
      'infra': { label: 'Infrastructure', keywords: ['deploy', 'docker', 'server', 'infra', 'hosting'] },
    },
  },
  'design': {
    label: 'Design',
    description: 'Canvas art, 3D graphics, themes, and visual asset creation',
    subcategories: {
      '3d': { label: '3D & WebGL', keywords: ['threejs', 'three', 'webgl', '3d', 'shader', 'geometry', 'lighting', 'material', 'texture'] },
      'canvas': { label: 'Canvas & Generative Art', keywords: ['canvas', 'generative', 'algorithmic', 'art', 'svg'] },
      'ui': { label: 'UI & Themes', keywords: ['theme', 'interface', 'design', 'gif', 'icon'] },
    },
  },
  'mobile': {
    label: 'Mobile',
    description: 'React Native, Expo, and native mobile development',
    subcategories: {
      'expo': { label: 'Expo', keywords: ['expo'] },
      'react-native': { label: 'React Native', keywords: ['react-native', 'native'] },
    },
  },
  'general': {
    label: 'General',
    description: 'Uncategorized and cross-domain skills',
    subcategories: {
      'utilities': { label: 'Utilities', keywords: ['util', 'helper', 'tool'] },
      'frameworks': { label: 'Framework Skills', keywords: ['next', 'angular', 'svelte', 'framework'] },
      'other': { label: 'Other', keywords: [] },
    },
  },
};

interface SkillRow {
  id: string;
  name: string;
  description: string | null;
  category: string | null;
  installs: number | null;
  review_count: number;
  avg_score: number | null;
  avg_security_score: number | null;
  rank_score: number | null;
}

/**
 * Assign a subcategory to a skill based on keyword matching.
 */
function assignSubcategory(skill: SkillRow, category: string): string {
  const cat = TAXONOMY[category];
  if (!cat) return 'other';

  // Match on name + ID only — descriptions often have generic boilerplate that causes false matches
  const text = `${skill.name} ${skill.id}`.toLowerCase();

  for (const [subKey, sub] of Object.entries(cat.subcategories)) {
    if (sub.keywords.length === 0) continue;
    if (sub.keywords.some(kw => text.includes(kw))) {
      return subKey;
    }
  }

  // Fallback: last subcategory (usually 'other' or 'tools')
  const keys = Object.keys(cat.subcategories);
  return keys[keys.length - 1];
}

/**
 * Format a skill as a one-line summary.
 */
function skillLine(s: SkillRow): string {
  const score = s.avg_score ? ` [${s.avg_score.toFixed(1)}/10]` : '';
  const security = s.avg_security_score ? ` [sec:${s.avg_security_score.toFixed(1)}]` : '';
  const installs = s.installs ? ` (${s.installs.toLocaleString()} installs)` : '';
  const desc = s.description ? ` — ${s.description.substring(0, 80)}` : '';
  return `- ${s.id}${score}${security}${installs}${desc}`;
}

/**
 * Format a skill with full details.
 */
function skillFull(s: SkillRow): string {
  let out = `### ${s.name}\n`;
  out += `ID: ${s.id}\n`;
  if (s.description) out += `Description: ${s.description}\n`;
  out += `Installs: ${s.installs ? s.installs.toLocaleString() : '0'}\n`;
  out += `Reviews: ${s.review_count}\n`;
  if (s.avg_score) out += `Score: ${s.avg_score.toFixed(1)}/10\n`;
  if (s.avg_security_score) out += `Security: ${s.avg_security_score.toFixed(1)}/10\n`;
  out += `Install: npx @gathers/skills add ${s.id}\n`;
  out += `Detail: /api/skills/${s.id}\n`;
  return out;
}

// ─── GET /api/browse ──────────────────────────────────────────────
// Root: show all categories with counts and subcategory summaries
router.get('/', (req, res) => {
  const db = getDb();

  const catCounts = db.prepare(`
    SELECT category, COUNT(*) as count
    FROM skills WHERE category IS NOT NULL
    GROUP BY category ORDER BY count DESC
  `).all() as Array<{ category: string; count: number }>;

  const totalSkills = (db.prepare('SELECT COUNT(*) as c FROM skills').get() as { c: number }).c;

  let text = `# Gather Skills Library\n`;
  text += `> ${totalSkills} skills across ${catCounts.length} categories. Navigate by drilling into a category.\n\n`;
  text += `## Categories\n\n`;

  for (const { category, count } of catCounts) {
    const tax = TAXONOMY[category];
    const label = tax?.label || category;
    const desc = tax?.description || '';
    const subKeys = tax ? Object.keys(tax.subcategories) : [];
    const subList = subKeys.length > 0 ? ` → ${subKeys.join(', ')}` : '';
    text += `- **${label}** (${count} skills): ${desc}${subList}\n`;
    text += `  Browse: /api/browse/${category}\n`;
  }

  text += `\n## Navigation\n`;
  text += `- /api/browse/:category — subcategories and skill summaries\n`;
  text += `- /api/browse/:category/:subcategory — skills in a subcategory\n`;
  text += `- Add ?detail=full for complete skill info\n`;
  text += `- Add ?start=N&end=M to paginate (default: first 30)\n`;

  res.type('text/plain').send(text);
});

// ─── GET /api/browse/:category ────────────────────────────────────
// Show subcategories with skill counts, or list all skills in the category
router.get('/:category', (req, res) => {
  const db = getDb();
  const category = req.params.category;
  const detail = req.query.detail as string || 'summary';
  const start = Math.max(0, parseInt(req.query.start as string) || 0);
  const end = Math.min(start + 100, parseInt(req.query.end as string) || start + 30);

  const tax = TAXONOMY[category];
  if (!tax) {
    return res.status(404).type('text/plain').send(`Category "${category}" not found.\n\nValid categories: ${Object.keys(TAXONOMY).join(', ')}`);
  }

  const skills = db.prepare(`
    SELECT id, name, description, category, installs, review_count, avg_score, avg_security_score, rank_score
    FROM skills WHERE category = ?
    ORDER BY installs DESC NULLS LAST, review_count DESC
  `).all(category) as SkillRow[];

  // Group by subcategory
  const groups: Record<string, SkillRow[]> = {};
  for (const sub of Object.keys(tax.subcategories)) {
    groups[sub] = [];
  }

  for (const skill of skills) {
    const sub = assignSubcategory(skill, category);
    if (!groups[sub]) groups[sub] = [];
    groups[sub].push(skill);
  }

  let text = `# ${tax.label}\n`;
  text += `> ${skills.length} skills. ${tax.description}\n\n`;

  // Show subcategory index
  text += `## Subcategories\n\n`;
  for (const [subKey, subVal] of Object.entries(tax.subcategories)) {
    const count = groups[subKey]?.length || 0;
    if (count === 0) continue;
    text += `- **${subVal.label}** (${count} skills) → /api/browse/${category}/${subKey}\n`;
  }

  // Show skills (paginated)
  const slice = skills.slice(start, end);
  text += `\n## Skills ${start + 1}–${start + slice.length} of ${skills.length}\n\n`;

  if (detail === 'full') {
    for (const s of slice) {
      text += skillFull(s) + '\n';
    }
  } else {
    for (const s of slice) {
      text += skillLine(s) + '\n';
    }
  }

  if (end < skills.length) {
    text += `\n→ Next: /api/browse/${category}?start=${end}&end=${end + 30}\n`;
  }

  res.type('text/plain').send(text);
});

// ─── GET /api/browse/:category/:subcategory ───────────────────────
// Show all skills in a subcategory
router.get('/:category/:subcategory', (req, res) => {
  const db = getDb();
  const category = req.params.category;
  const subcategory = req.params.subcategory;
  const detail = req.query.detail as string || 'summary';
  const start = Math.max(0, parseInt(req.query.start as string) || 0);
  const end = Math.min(start + 100, parseInt(req.query.end as string) || start + 30);

  const tax = TAXONOMY[category];
  if (!tax) {
    return res.status(404).type('text/plain').send(`Category "${category}" not found.`);
  }

  const subDef = tax.subcategories[subcategory];
  if (!subDef) {
    return res.status(404).type('text/plain').send(
      `Subcategory "${subcategory}" not found in ${category}.\n\nValid: ${Object.keys(tax.subcategories).join(', ')}`
    );
  }

  const allSkills = db.prepare(`
    SELECT id, name, description, category, installs, review_count, avg_score, avg_security_score, rank_score
    FROM skills WHERE category = ?
    ORDER BY installs DESC NULLS LAST, review_count DESC
  `).all(category) as SkillRow[];

  // Filter to this subcategory
  const skills = allSkills.filter(s => assignSubcategory(s, category) === subcategory);

  const slice = skills.slice(start, end);

  let text = `# ${tax.label} → ${subDef.label}\n`;
  text += `> ${skills.length} skills\n\n`;

  if (detail === 'full') {
    for (const s of slice) {
      text += skillFull(s) + '\n';
    }
  } else {
    for (const s of slice) {
      text += skillLine(s) + '\n';
    }
  }

  if (end < skills.length) {
    text += `\n→ Next: /api/browse/${category}/${subcategory}?start=${end}&end=${end + 30}\n`;
  }

  text += `\n← Back: /api/browse/${category}\n`;

  res.type('text/plain').send(text);
});

export default router;
