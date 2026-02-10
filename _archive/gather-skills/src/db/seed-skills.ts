import 'dotenv/config';
import { initDb, getDb, closeDb, getDbPath } from './index.js';

const SKILLS = [
  { id: "anthropics/algorithmic-art", name: "Algorithmic Art", description: "Create generative art using p5.js with seeded randomness, flow fields, and particle systems", source: "github" },
  { id: "anthropics/brand-guidelines", name: "Brand Guidelines", description: "Apply Anthropic's official brand colors and typography to artifacts", source: "github" },
  { id: "anthropics/canvas-design", name: "Canvas Design", description: "Create beautiful visual art in .png and .pdf formats using design philosophies", source: "github" },
  { id: "anthropics/doc-coauthoring", name: "Doc Co-Authoring", description: "Structured workflow for co-authoring documentation, proposals, and technical specs through collaborative iteration", source: "github" },
  { id: "anthropics/docx", name: "Word Documents", description: "Create, read, edit, and manipulate Word documents with support for tracked changes, comments, and formatting", source: "github" },
  { id: "anthropics/frontend-design", name: "Frontend Design", description: "Create distinctive, production-grade frontend interfaces with bold design decisions using React and Tailwind", source: "skills.sh" },
  { id: "anthropics/internal-comms", name: "Internal Comms", description: "Write internal communications like status reports, newsletters, FAQs, and incident reports", source: "github" },
  { id: "anthropics/mcp-builder", name: "MCP Builder", description: "Guide for creating high-quality MCP servers to integrate external APIs and services", source: "skills.sh" },
  { id: "anthropics/pdf", name: "PDF Tools", description: "Extract text and tables, create, merge, split, rotate, watermark, and fill forms in PDF documents", source: "skills.sh" },
  { id: "anthropics/pptx", name: "PowerPoint Presentations", description: "Create, edit, and analyze PowerPoint presentations with support for layouts, templates, charts, and slide generation", source: "skills.sh" },
  { id: "anthropics/skill-creator", name: "Skill Creator", description: "Interactive guide for creating new skills that extend Claude's capabilities with specialized knowledge and workflows", source: "skills.sh" },
  { id: "anthropics/slack-gif-creator", name: "Slack GIF Creator", description: "Create animated GIFs optimized for Slack's size constraints with validation and animation concepts", source: "github" },
  { id: "anthropics/theme-factory", name: "Theme Factory", description: "Style artifacts with 10 pre-set professional themes including color palettes and font pairings", source: "github" },
  { id: "anthropics/web-artifacts-builder", name: "Web Artifacts Builder", description: "Build complex claude.ai HTML artifacts using React, Tailwind CSS, and shadcn/ui components", source: "github" },
  { id: "anthropics/webapp-testing", name: "Web App Testing", description: "Test local web applications using Playwright for UI verification, debugging, and browser screenshots", source: "skills.sh" },
  { id: "anthropics/xlsx", name: "Excel Spreadsheets", description: "Create, edit, and analyze Excel spreadsheets with support for formulas, formatting, and data visualization", source: "skills.sh" },
  { id: "vercel-labs/find-skills", name: "Find Skills", description: "Discover and install agent skills when looking for functionality that might exist as an installable skill", source: "skills.sh" },
  { id: "vercel-labs/vercel-react-best-practices", name: "Vercel React Best Practices", description: "React and Next.js performance optimization guidelines from Vercel Engineering for writing and reviewing code", source: "skills.sh" },
  { id: "vercel-labs/web-design-guidelines", name: "Web Design Guidelines", description: "Review UI code for Web Interface Guidelines compliance including accessibility and UX best practices", source: "skills.sh" },
  { id: "vercel-labs/vercel-composition-patterns", name: "Vercel Composition Patterns", description: "React composition patterns that scale for building flexible component libraries and reusable APIs", source: "skills.sh" },
  { id: "vercel-labs/agent-browser", name: "Agent Browser", description: "Browser automation capabilities for AI agents to navigate and interact with web pages", source: "skills.sh" },
  { id: "vercel-labs/vercel-react-native-skills", name: "Vercel React Native Skills", description: "React Native and Expo best practices for building performant mobile apps with animations and native modules", source: "skills.sh" },
  { id: "vercel-labs/next-best-practices", name: "Next.js Best Practices", description: "Next.js development best practices from Vercel for routing, data fetching, and optimization", source: "skills.sh" },
  { id: "remotion-dev/remotion-best-practices", name: "Remotion Best Practices", description: "Best practices for Remotion video creation in React including animations and compositions", source: "skills.sh" },
  { id: "browser-use/browser-use", name: "Browser Use", description: "Automate browser interactions for web testing, form filling, screenshots, and data extraction", source: "skills.sh" },
  { id: "supabase/supabase-postgres-best-practices", name: "Supabase Postgres Best Practices", description: "Postgres performance optimization and best practices for queries, schema design, and database configuration", source: "skills.sh" },
  { id: "expo/building-native-ui", name: "Building Native UI", description: "Complete guide for building beautiful apps with Expo Router covering styling, components, navigation, and animations", source: "skills.sh" },
  { id: "obra/brainstorming", name: "Brainstorming", description: "Explore user intent, requirements, and design through collaborative dialogue before any creative work", source: "skills.sh" },
  { id: "obra/systematic-debugging", name: "Systematic Debugging", description: "Structured approach to debugging bugs and test failures before proposing fixes, avoiding random patches", source: "skills.sh" },
  { id: "obra/test-driven-development", name: "Test-Driven Development", description: "Write the test first, watch it fail, write minimal code to pass for any feature or bugfix", source: "skills.sh" },
  { id: "obra/subagent-driven-development", name: "Subagent-Driven Development", description: "Execute implementation plans by dispatching fresh subagents per task with two-stage review", source: "github" },
  { id: "obra/dispatching-parallel-agents", name: "Dispatching Parallel Agents", description: "Dispatch parallel agents for 2+ independent tasks that can be worked on without shared state", source: "github" },
  { id: "obra/verification-before-completion", name: "Verification Before Completion", description: "Run verification commands and confirm output before making any success claims on completed work", source: "github" },
  { id: "obra/writing-plans", name: "Writing Plans", description: "Write comprehensive implementation plans with bite-sized tasks before touching code", source: "github" },
  { id: "obra/requesting-code-review", name: "Requesting Code Review", description: "Dispatch code reviewer subagent to catch issues before they cascade when completing tasks or features", source: "github" },
  { id: "coreyhaines31/seo-audit", name: "SEO Audit", description: "Audit, review, and diagnose SEO issues including technical SEO, on-page SEO, and meta tags", source: "skills.sh" },
  { id: "coreyhaines31/copywriting", name: "Copywriting", description: "Write, rewrite, or improve marketing copy for homepages, landing pages, pricing pages, and CTAs", source: "skills.sh" },
  { id: "coreyhaines31/marketing-psychology", name: "Marketing Psychology", description: "Apply 70+ psychological principles, mental models, and behavioral science to marketing decisions", source: "skills.sh" },
  { id: "coreyhaines31/programmatic-seo", name: "Programmatic SEO", description: "Create SEO-driven pages at scale using templates and data for directories, locations, and comparisons", source: "skills.sh" },
  { id: "coreyhaines31/marketing-ideas", name: "Marketing Ideas", description: "139 proven marketing approaches organized by category for SaaS and software products", source: "skills.sh" },
  { id: "coreyhaines31/content-strategy", name: "Content Strategy", description: "Plan content strategy, decide what to create, and organize topic clusters for blogs and publications", source: "github" },
  { id: "trailofbits/static-analysis", name: "Static Analysis", description: "Security-focused static analysis with CodeQL and Semgrep for vulnerability detection", source: "github" },
  { id: "trailofbits/variant-analysis", name: "Variant Analysis", description: "Find variants of known vulnerabilities across codebases using pattern-based security analysis", source: "github" },
  { id: "trailofbits/semgrep-rule-creator", name: "Semgrep Rule Creator", description: "Create custom Semgrep rules for detecting security patterns and vulnerabilities in code", source: "github" },
  { id: "better-auth/better-auth-best-practices", name: "Better Auth Best Practices", description: "Authentication best practices and patterns for implementing secure auth flows with Better Auth", source: "skills.sh" },
];

console.log(`Seeding ${SKILLS.length} skills into database at: ${getDbPath()}`);
initDb();

const db = getDb();
const insert = db.prepare(`
  INSERT OR IGNORE INTO skills (id, name, description, source)
  VALUES (?, ?, ?, ?)
`);

const insertMany = db.transaction((skills: typeof SKILLS) => {
  let inserted = 0;
  let skipped = 0;
  for (const skill of skills) {
    const result = insert.run(skill.id, skill.name, skill.description, skill.source);
    if (result.changes > 0) {
      inserted++;
    } else {
      skipped++;
    }
  }
  return { inserted, skipped };
});

const { inserted, skipped } = insertMany(SKILLS);
console.log(`Done: ${inserted} inserted, ${skipped} already existed`);

const total = (db.prepare('SELECT COUNT(*) as c FROM skills').get() as { c: number }).c;
console.log(`Total skills in database: ${total}`);

closeDb();
