import 'dotenv/config';
import { initDb, getDb, closeDb, getDbPath } from '../db/index.js';

interface RawSkill {
  source: string;
  skillId: string;
  name: string;
  installs: number;
}

const CATEGORY_RULES: Array<{ category: string; keywords: RegExp }> = [
  { category: 'frontend', keywords: /react|vue|angular|svelte|next[\-.]?js|nuxt|tailwind|css|html|ui|component|shadcn|web-design|frontend|remotion/i },
  { category: 'backend', keywords: /api|database|postgres|supabase|redis|graphql|server|express|django|auth|better-auth/i },
  { category: 'devtools', keywords: /git|testing|debug|code-review|refactor|lint|ci[\-.]?cd|docker|k8s|playwright|verification|test-driven|subagent|parallel-agent|plan/i },
  { category: 'security', keywords: /security|semgrep|vulnerab|attack|analysis|threat|audit|pentest|static-analysis|variant-analysis|trail[\-.]?of[\-.]?bits/i },
  { category: 'ai-agents', keywords: /mcp|agent|llm|prompt|rag|inference|claude|openai|langchain|skill-creator|find-skills|browser-use/i },
  { category: 'mobile', keywords: /react[\-.]?native|expo|native[\-.]?ui|ios|android|swift/i },
  { category: 'content', keywords: /copywriting|marketing|seo|content|blog|article|slide|presentation|pdf|docx|pptx|xlsx|doc-co|internal-comms|brainstorm/i },
  { category: 'design', keywords: /design|canvas|art|animation|3d|threejs|remotion|gif|theme|brand|visual|algorithmic/i },
  { category: 'data', keywords: /data|analytics|csv|scrap|observ|monitor|logging/i },
];

function categorize(skillId: string, source: string): string {
  const text = `${skillId} ${source}`;
  for (const rule of CATEGORY_RULES) {
    if (rule.keywords.test(text)) {
      return rule.category;
    }
  }
  return 'general';
}

async function scrape(): Promise<RawSkill[]> {
  console.log('Fetching skills.sh...');
  const res = await fetch('https://skills.sh');
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const html = await res.text();

  // The data is in an RSC payload inside self.__next_f.push([1,"..."])
  // with JSON-escaped quotes: \"source\":\"vercel-labs/skills\"
  // Strategy: find the escaped initialSkills array and parse via JSON.parse on the outer string

  // Match the escaped array: \"initialSkills\":[{...},...,{...}]
  const pattern = /\\?"initialSkills\\?":\s*(\[.*?\])\s*[,}\\]/;
  let match = html.match(pattern);

  if (match) {
    // The array content has escaped quotes — unescape them
    const raw = match[1].replace(/\\"/g, '"');
    const skills: RawSkill[] = JSON.parse(raw);
    console.log(`Parsed ${skills.length} skills from skills.sh`);
    return skills;
  }

  // Fallback: extract individual escaped skill objects
  const chunks = html.match(/\{\\"source\\":\\"[^"]+\\",\\"skillId\\":\\"[^"]+\\",\\"name\\":\\"[^"]+\\",\\"installs\\":\d+\}/g);
  if (chunks && chunks.length > 10) {
    const unescaped = `[${chunks.join(',')}]`.replace(/\\"/g, '"');
    const arr: RawSkill[] = JSON.parse(unescaped);
    console.log(`Extracted ${arr.length} skills via chunk matching`);
    return arr;
  }

  throw new Error('Could not find initialSkills data in skills.sh response');
}

async function main() {
  const rawSkills = await scrape();

  console.log(`Upserting into database at: ${getDbPath()}`);
  initDb();
  const db = getDb();

  const upsert = db.prepare(`
    INSERT INTO skills (id, name, description, source, category, installs)
    VALUES (?, ?, ?, 'skills.sh', ?, ?)
    ON CONFLICT(id) DO UPDATE SET
      installs = excluded.installs,
      category = COALESCE(skills.category, excluded.category)
  `);

  const upsertMany = db.transaction((skills: RawSkill[]) => {
    let inserted = 0;
    let updated = 0;
    for (const skill of skills) {
      const id = `${skill.source}/${skill.skillId}`;
      const category = categorize(skill.skillId, skill.source);
      const result = upsert.run(id, skill.skillId, null, category, skill.installs);
      if (result.changes > 0) {
        // Check if it was insert or update
        const existing = db.prepare('SELECT created_at FROM skills WHERE id = ?').get(id);
        if (existing) updated++; else inserted++;
      }
    }
    return { inserted, updated, total: skills.length };
  });

  const result = upsertMany(rawSkills);
  console.log(`Done: ${result.total} skills processed`);

  // Also categorize any existing skills that lack a category
  const uncategorized = db.prepare('SELECT id, name FROM skills WHERE category IS NULL').all() as Array<{ id: string; name: string }>;
  if (uncategorized.length > 0) {
    const updateCat = db.prepare('UPDATE skills SET category = ? WHERE id = ?');
    for (const skill of uncategorized) {
      const category = categorize(skill.name, skill.id);
      updateCat.run(category, skill.id);
    }
    console.log(`Categorized ${uncategorized.length} existing skills`);
  }

  // Print category breakdown
  const categories = db.prepare('SELECT category, COUNT(*) as c FROM skills GROUP BY category ORDER BY c DESC').all() as Array<{ category: string; c: number }>;
  console.log('\nCategory breakdown:');
  for (const cat of categories) {
    console.log(`  ${cat.category}: ${cat.c}`);
  }

  const total = (db.prepare('SELECT COUNT(*) as c FROM skills').get() as { c: number }).c;
  console.log(`\nTotal skills in database: ${total}`);

  closeDb();
}

main().catch((err) => {
  console.error('Scrape failed:', err);
  process.exit(1);
});
