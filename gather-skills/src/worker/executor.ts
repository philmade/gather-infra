import { spawn } from 'child_process';
import { mkdtempSync, rmSync, readdirSync, readFileSync, existsSync, mkdirSync, writeFileSync, statSync } from 'fs';
import { tmpdir } from 'os';
import { join, extname, relative, dirname } from 'path';
import { nanoid } from 'nanoid';
import { getDb, getDbPath } from '../db/index.js';
import { updateSkillRanking } from '../lib/ranking.js';
import { createAttestation } from '../lib/attestation.js';
import type { Review, ReviewResult, ExecutionLogs } from '../lib/types.js';

const MOCK_RESPONSE = `
## Mock Review

Installing skill... done.
Reading skill files for security review...
Executing task... done.
Analyzing results...

The skill performed well overall. Here's my assessment:

{
  "score": 8,
  "what_worked": "Clean installation process, good documentation, skill executed the task successfully and produced expected output.",
  "what_failed": "Minor issue with error handling for edge cases.",
  "skill_feedback": "Consider adding more examples in the README and improving error messages for invalid inputs.",
  "security_score": 9,
  "security_notes": "No system writes, no network calls, no credential access. All operations stay within project scope."
}
`;

const REVIEW_PROMPT = `You are reviewing a Claude Code skill. Your review has two parts: a quality review and a security review.

## Part 1: Install and Test

1. Install the skill using: claude skill add {skill_id}
2. Execute the skill with the given task
3. Evaluate how well it worked

## Part 1b: Build Output

If the skill produces buildable output (a web app, documents, reports, etc.), run the appropriate build command (npm run build, make, etc.). A skill that claims to build something but doesn't include build instructions should score lower on quality.

## Part 2: Security Review

After installing, READ every file that was installed as part of the skill — the SKILL.md and any supporting scripts, templates, or config files. Read them completely. Then assess security:

Start at a security_score of 10 (clean) and subtract based on what you find:

- File writes outside project (writes to ~/.zshrc, ~/.bashrc, ~/.config/, system dirs): -2 to -4
- Credential access (reads browser cookies, Keychain, SSH keys, .env files, API tokens): -4 to -8
- Network calls (fetches from third-party URLs, posts data externally): -1 to -3
- Shell commands that install system-wide software (curl|sh, pip install, npm install -g, brew install): -2 to -4
- Persistence (cron jobs, launchd agents, PATH modifications, shell aliases that survive the session): -3 to -5
- Obfuscation (base64 payloads, hex-encoded strings, intentionally vague instructions, hidden behavior): -5 to -8

If the skill only reads/writes within the project directory and doesn't access credentials or make network calls, it scores 10.

In security_notes, be specific: name the exact files, lines, commands, or patterns you found. If clean, say "No security concerns found. All operations stay within project scope."

## Output

After both parts, provide your assessment in this exact JSON format:
{
  "score": <number 1-10, quality score for how well the skill worked>,
  "what_worked": "<specific description of what worked well>",
  "what_failed": "<specific issues or failures, or 'Nothing' if all worked>",
  "skill_feedback": "<constructive feedback for the skill author>",
  "security_score": <number 1-10, based on the security review above>,
  "security_notes": "<specific findings from reading the skill files, or 'No security concerns found' if clean>"
}

IMPORTANT: Your response MUST end with a valid JSON block containing ALL six fields.
`;

export interface ExecuteOptions {
  stream?: boolean;
  dangerous?: boolean;  // Skip all permission prompts
  runnerType?: string;  // claude, aider, goose, etc.
}

export interface ExecuteResult {
  reviewId: string;
  status: 'complete' | 'failed';
  score?: number;
  what_worked?: string;
  what_failed?: string;
  skill_feedback?: string;
  security_score?: number;
  security_notes?: string;
  execution_time_ms: number;
  cli_output: string;
  artifacts: Array<{
    file_name: string;
    mime_type: string;
    content: Buffer;
  }>;
  proof?: {
    id: string;
    signature: string;
    public_key: string;
  };
}

export async function executeReview(
  reviewId: string,
  skillId: string,
  task: string,
  options: ExecuteOptions = {}
): Promise<ExecuteResult> {
  const db = getDb();
  const workDir = mkdtempSync(join(tmpdir(), 'reskill-'));
  const startTime = Date.now();
  const runnerType = options.runnerType || 'claude';
  const permissionMode = options.dangerous ? 'dangerous' : 'default';

  try {
    // Update status to running
    db.prepare('UPDATE reviews SET status = ?, runner_type = ?, permission_mode = ? WHERE id = ?')
      .run('running', runnerType, permissionMode, reviewId);

    // Build the full prompt
    const fullPrompt = `${REVIEW_PROMPT}

Skill to review: ${skillId}
Task: ${task}

Begin by installing the skill and then executing the task.`;

    // Execute claude -p (with timeout)
    const { output, exitCode, timedOut } = await runClaude(fullPrompt, workDir, {
      stream: options.stream,
      dangerous: options.dangerous,
    });

    const executionTime = Date.now() - startTime;

    // Handle timeout
    if (timedOut) {
      db.prepare(`
        UPDATE reviews
        SET status = ?, cli_output = ?, execution_time_ms = ?
        WHERE id = ?
      `).run('failed', output, executionTime, reviewId);

      cleanupWorkDir(workDir);

      return {
        reviewId,
        status: 'failed',
        execution_time_ms: executionTime,
        cli_output: output,
        artifacts: [],
      };
    }

    // Parse the result
    const result = parseReviewResult(output);

    // Capture artifacts from working directory
    const artifacts = collectArtifacts(workDir);

    if (result) {
      // Update review with results
      db.prepare(`
        UPDATE reviews
        SET status = ?, score = ?, what_worked = ?, what_failed = ?,
            skill_feedback = ?, security_score = ?, security_notes = ?,
            agent_model = ?, execution_time_ms = ?, cli_output = ?
        WHERE id = ?
      `).run(
        'complete',
        result.score,
        result.what_worked,
        result.what_failed,
        result.skill_feedback,
        result.security_score,
        result.security_notes,
        'claude-sonnet', // Default model
        executionTime,
        output,
        reviewId
      );

      // Generate attestation/proof
      const attestation = createAttestation({
        skill_id: skillId,
        task,
        cli_output: output,
        score: result.score,
        what_worked: result.what_worked,
        what_failed: result.what_failed,
        execution_time_ms: executionTime,
      });

      // Store proof
      db.prepare(`
        INSERT INTO proofs (id, review_id, claim_data, identifier, signatures, witnesses, verified)
        VALUES (?, ?, ?, ?, ?, ?, 1)
      `).run(
        attestation.id,
        reviewId,
        JSON.stringify(attestation.payload),
        attestation.execution_hash,
        JSON.stringify([attestation.signature]),
        JSON.stringify([{ type: 'ed25519', public_key: attestation.public_key }])
      );

      // Link proof to review
      db.prepare('UPDATE reviews SET proof_id = ? WHERE id = ?').run(attestation.id, reviewId);

      // Store artifacts to filesystem
      if (artifacts.length > 0) {
        const dataDir = dirname(getDbPath());
        const artifactDir = join(dataDir, 'artifacts', reviewId);
        for (const artifact of artifacts) {
          const artifactId = nanoid();
          const destPath = join(artifactDir, artifact.file_name);
          mkdirSync(dirname(destPath), { recursive: true });
          writeFileSync(destPath, artifact.content);
          const filePath = `artifacts/${reviewId}/${artifact.file_name}`;
          db.prepare(`
            INSERT INTO artifacts (id, review_id, file_name, mime_type, file_path, size_bytes)
            VALUES (?, ?, ?, ?, ?, ?)
          `).run(artifactId, reviewId, artifact.file_name, artifact.mime_type, filePath, artifact.content.length);
        }
      }

      // Update skill stats
      updateSkillStats(db, skillId);

      // Cleanup working directory
      cleanupWorkDir(workDir);

      return {
        reviewId,
        status: 'complete',
        score: result.score,
        what_worked: result.what_worked,
        what_failed: result.what_failed,
        skill_feedback: result.skill_feedback,
        security_score: result.security_score ?? undefined,
        security_notes: result.security_notes ?? undefined,
        execution_time_ms: executionTime,
        cli_output: output,
        artifacts,
        proof: {
          id: attestation.id,
          signature: attestation.signature,
          public_key: attestation.public_key,
        },
      };
    } else {
      // Failed to parse result
      db.prepare(`
        UPDATE reviews
        SET status = ?, cli_output = ?, execution_time_ms = ?
        WHERE id = ?
      `).run('failed', output, executionTime, reviewId);

      cleanupWorkDir(workDir);

      return {
        reviewId,
        status: 'failed',
        execution_time_ms: executionTime,
        cli_output: output,
        artifacts,
      };
    }
  } catch (error) {
    const errorMsg = error instanceof Error ? error.message : String(error);
    db.prepare(`
      UPDATE reviews
      SET status = ?, cli_output = ?
      WHERE id = ?
    `).run('failed', `Error: ${errorMsg}`, reviewId);

    cleanupWorkDir(workDir);

    return {
      reviewId,
      status: 'failed',
      execution_time_ms: Date.now() - startTime,
      cli_output: `Error: ${errorMsg}`,
      artifacts: [],
    };
  }
}

function cleanupWorkDir(workDir: string): void {
  try {
    rmSync(workDir, { recursive: true, force: true });
  } catch {
    // Ignore cleanup errors
  }
}

// Common file extensions to capture as artifacts
const ARTIFACT_EXTENSIONS = [
  '.pptx', '.pdf', '.docx', '.xlsx',  // Office docs
  '.html', '.css', '.js', '.ts',       // Web files
  '.json', '.xml', '.yaml', '.yml',    // Data files
  '.png', '.jpg', '.jpeg', '.gif', '.svg',  // Images
  '.md', '.txt',                       // Text
  '.zip', '.tar', '.gz',               // Archives
];

const MIME_TYPES: Record<string, string> = {
  '.pptx': 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  '.pdf': 'application/pdf',
  '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  '.xlsx': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  '.html': 'text/html',
  '.css': 'text/css',
  '.js': 'application/javascript',
  '.ts': 'application/typescript',
  '.json': 'application/json',
  '.xml': 'application/xml',
  '.yaml': 'application/yaml',
  '.yml': 'application/yaml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.svg': 'image/svg+xml',
  '.md': 'text/markdown',
  '.txt': 'text/plain',
  '.zip': 'application/zip',
  '.tar': 'application/x-tar',
  '.gz': 'application/gzip',
};

const SKIP_DIRS = new Set(['node_modules', '.git', '.claude', '__pycache__', '.next', '.cache', '.venv', 'venv']);
const MAX_TOTAL_BYTES = 50 * 1024 * 1024; // 50MB total
const MAX_FILE_BYTES = 10 * 1024 * 1024;  // 10MB per file
const MAX_FILE_COUNT = 200;

function collectArtifacts(workDir: string): Array<{ file_name: string; mime_type: string; content: Buffer }> {
  const artifacts: Array<{ file_name: string; mime_type: string; content: Buffer }> = [];
  let totalBytes = 0;

  function walk(dir: string): void {
    if (artifacts.length >= MAX_FILE_COUNT) return;

    let entries;
    try {
      entries = readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }

    for (const entry of entries) {
      if (artifacts.length >= MAX_FILE_COUNT || totalBytes >= MAX_TOTAL_BYTES) return;

      if (entry.isDirectory()) {
        if (SKIP_DIRS.has(entry.name)) continue;
        walk(join(dir, entry.name));
        continue;
      }

      if (!entry.isFile()) continue;

      const ext = extname(entry.name).toLowerCase();
      if (!ARTIFACT_EXTENSIONS.includes(ext)) continue;

      const filePath = join(dir, entry.name);
      let content: Buffer;
      try {
        content = readFileSync(filePath);
      } catch {
        continue;
      }

      if (content.length > MAX_FILE_BYTES) continue;
      if (totalBytes + content.length > MAX_TOTAL_BYTES) continue;

      totalBytes += content.length;
      const relPath = relative(workDir, filePath);

      artifacts.push({
        file_name: relPath,
        mime_type: MIME_TYPES[ext] || 'application/octet-stream',
        content,
      });
    }
  }

  try {
    walk(workDir);
  } catch {
    // Ignore errors
  }

  return artifacts;
}

// Default timeout: 5 minutes. Override with RESKILL_REVIEW_TIMEOUT_MS env var.
const DEFAULT_TIMEOUT_MS = 5 * 60 * 1000;

interface RunClaudeOptions {
  stream?: boolean;
  dangerous?: boolean;
  timeoutMs?: number;
}

async function runClaude(
  prompt: string,
  cwd: string,
  options: RunClaudeOptions = {}
): Promise<{ output: string; exitCode: number; timedOut?: boolean }> {
  const { stream = false, dangerous = false } = options;
  const envTimeout = parseInt(process.env.RESKILL_REVIEW_TIMEOUT_MS || '', 10) || DEFAULT_TIMEOUT_MS;
  const timeoutMs = options.timeoutMs ?? envTimeout;

  // Mock mode for testing
  if (process.env.RESKILL_MOCK) {
    if (stream) {
      // Simulate streaming output
      for (const line of MOCK_RESPONSE.split('\n')) {
        process.stdout.write(line + '\n');
        await new Promise(r => setTimeout(r, 50));
      }
    }
    return { output: MOCK_RESPONSE, exitCode: 0 };
  }

  return new Promise((resolve) => {
    const chunks: Buffer[] = [];
    let resolved = false;

    // Build claude args
    const args = ['-p', '-'];
    if (dangerous) {
      args.unshift('--dangerously-skip-permissions');
    }

    // Use stdin for prompt to avoid shell escaping issues with long prompts
    const proc = spawn('claude', args, {
      cwd,
      stdio: ['pipe', 'pipe', 'pipe'],
    });

    // Write prompt to stdin
    proc.stdin.write(prompt);
    proc.stdin.end();

    // Set up timeout
    const timer = setTimeout(() => {
      if (resolved) return;
      resolved = true;
      const partialOutput = Buffer.concat(chunks).toString('utf-8');
      // Kill the process tree
      try {
        proc.kill('SIGTERM');
        // Force kill after 5 seconds if SIGTERM didn't work
        setTimeout(() => {
          try { proc.kill('SIGKILL'); } catch { /* already dead */ }
        }, 5000);
      } catch { /* process already exited */ }
      resolve({
        output: partialOutput + `\n\n[TIMEOUT] Review killed after ${Math.round(timeoutMs / 1000)}s`,
        exitCode: 1,
        timedOut: true,
      });
    }, timeoutMs);

    proc.stdout.on('data', (data) => {
      if (stream) process.stdout.write(data);
      chunks.push(data);
    });
    proc.stderr.on('data', (data) => {
      if (stream) process.stderr.write(data);
      chunks.push(data);
    });

    proc.on('close', (code) => {
      clearTimeout(timer);
      if (resolved) return;
      resolved = true;
      resolve({
        output: Buffer.concat(chunks).toString('utf-8'),
        exitCode: code ?? 1,
      });
    });

    proc.on('error', (err) => {
      clearTimeout(timer);
      if (resolved) return;
      resolved = true;
      resolve({
        output: `Failed to spawn claude: ${err.message}`,
        exitCode: 1,
      });
    });
  });
}

function parseReviewResult(output: string): ReviewResult | null {
  // Find JSON block in output
  const jsonMatch = output.match(/\{[\s\S]*"score"[\s\S]*"what_worked"[\s\S]*"what_failed"[\s\S]*"skill_feedback"[\s\S]*\}/);

  if (!jsonMatch) {
    return null;
  }

  try {
    const parsed = JSON.parse(jsonMatch[0]);

    if (
      typeof parsed.score === 'number' &&
      typeof parsed.what_worked === 'string' &&
      typeof parsed.what_failed === 'string' &&
      typeof parsed.skill_feedback === 'string'
    ) {
      return {
        score: Math.min(10, Math.max(1, parsed.score)),
        what_worked: parsed.what_worked,
        what_failed: parsed.what_failed,
        skill_feedback: parsed.skill_feedback,
        security_score: typeof parsed.security_score === 'number' ? Math.min(10, Math.max(1, parsed.security_score)) : null,
        security_notes: typeof parsed.security_notes === 'string' ? parsed.security_notes : null,
      };
    }
  } catch {
    // JSON parse failed
  }

  return null;
}

function updateSkillStats(db: import('better-sqlite3').Database, skillId: string): void {
  // Update review count and avg score
  const stats = db.prepare(`
    SELECT COUNT(*) as count, AVG(score) as avg
    FROM reviews
    WHERE skill_id = ? AND status = 'complete' AND score IS NOT NULL
  `).get(skillId) as { count: number; avg: number | null };

  const secStats = db.prepare(`
    SELECT AVG(security_score) as avg
    FROM reviews
    WHERE skill_id = ? AND status = 'complete' AND security_score IS NOT NULL
  `).get(skillId) as { avg: number | null };

  db.prepare(`
    UPDATE skills
    SET review_count = ?, avg_score = ?, avg_security_score = ?
    WHERE id = ?
  `).run(stats.count, stats.avg, secStats.avg, skillId);

  // Update ranking
  updateSkillRanking(db, skillId);
}
