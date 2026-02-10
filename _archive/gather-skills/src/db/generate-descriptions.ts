/**
 * Generate descriptions for skills that don't have one.
 * Uses skill name, category, and owner to construct a meaningful one-liner.
 * Run: npx tsx src/db/generate-descriptions.ts
 */
import { getDb, initDb } from './index.js';

function generateDescription(skill: { id: string; name: string; category: string | null }): string {
  const name = skill.name;
  const category = skill.category || 'general';
  const parts = skill.id.split('/');
  const owner = parts[0];

  // Clean up skill name: convert dashes to spaces, capitalize
  const readable = name
    .replace(/-/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase());

  // Category-based description templates
  const templates: Record<string, (name: string, readable: string) => string> = {
    frontend: (n, r) => `${r} — a frontend development skill for building and styling web interfaces.`,
    backend: (n, r) => `${r} — a backend development skill for server-side logic and APIs.`,
    security: (n, r) => `${r} — a security skill for auditing, reviewing, and hardening code.`,
    content: (n, r) => `${r} — a content creation skill for generating documents, media, and written output.`,
    design: (n, r) => `${r} — a design skill for creating visual assets and UI components.`,
    devtools: (n, r) => `${r} — a developer tools skill for improving workflows and automation.`,
    'ai-agents': (n, r) => `${r} — an AI agent skill for extending Claude Code capabilities.`,
    general: (n, r) => `${r} — a skill for AI coding agents.`,
  };

  // Special cases for well-known skill names
  const specialCases: Record<string, string> = {
    'pdf': 'Generate, read, and manipulate PDF documents.',
    'pptx': 'Create and edit PowerPoint presentations.',
    'docx': 'Create and edit Word documents with docx-js.',
    'mcp-builder': 'Build Model Context Protocol (MCP) servers with best practices.',
    'frontend-design': 'Design and implement frontend interfaces with modern CSS and HTML.',
    'web-artifacts-builder': 'Build self-contained HTML artifacts with React, TypeScript, and Tailwind.',
    'seo': 'Audit and optimize websites for search engine performance.',
    'accessibility': 'Audit and improve web accessibility (WCAG compliance).',
    'performance': 'Profile and optimize web application performance.',
    'core-web-vitals': 'Monitor and improve Core Web Vitals metrics.',
    'best-practices': 'Apply web development best practices across your codebase.',
    'brainstorming': 'Structured brainstorming and ideation for product development.',
    'find-skills': 'Discover and install AI agent skills from the community.',
    'commit': 'Generate well-structured git commit messages.',
    'code-review': 'Perform thorough code reviews with actionable feedback.',
    'security-review': 'Review code for security vulnerabilities and risk patterns.',
    'tdd-workflow': 'Test-driven development workflow for writing tests first.',
  };

  // Check special cases first
  if (specialCases[name]) return specialCases[name];

  // Use category template
  const template = templates[category] || templates['general'];
  return template(name, readable);
}

// Main
initDb();
const db = getDb();

const skills = db.prepare('SELECT id, name, category FROM skills WHERE description IS NULL').all() as Array<{
  id: string;
  name: string;
  category: string | null;
}>;

console.log(`Generating descriptions for ${skills.length} skills...`);

const update = db.prepare('UPDATE skills SET description = ? WHERE id = ?');

let count = 0;
for (const skill of skills) {
  const desc = generateDescription(skill);
  update.run(desc, skill.id);
  count++;
}

console.log(`Updated ${count} skills with descriptions.`);
