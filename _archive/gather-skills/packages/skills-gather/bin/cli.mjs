#!/usr/bin/env node

import { execSync, spawn } from "child_process";
import { existsSync, mkdirSync, readdirSync, statSync, readFileSync, writeFileSync, cpSync, rmSync } from "fs";
import { join, basename, sep } from "path";
import { tmpdir, homedir } from "os";
import { randomBytes, createHash, generateKeyPairSync, sign, randomUUID } from "crypto";

// ── Config ──────────────────────────────────────────────────────────

const API = "https://skills.gather.is";
const GATHER_DIR = join(homedir(), ".gather");
const KEYS_PATH = join(GATHER_DIR, "keys.json");
const REVIEW_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes

// Agent install paths — where each agent looks for skills
const AGENTS = {
  "claude-code": { dir: ".claude/skills", global: join(homedir(), ".claude", "skills") },
  "cursor":      { dir: ".cursor/skills", global: join(homedir(), ".cursor", "skills") },
  "windsurf":    { dir: ".windsurf/skills", global: join(homedir(), ".windsurf", "skills") },
  "cline":       { dir: ".cline/skills", global: null },
  "roo":         { dir: ".roo/skills", global: join(homedir(), ".roo", "skills") },
};

// ── Review Prompt ───────────────────────────────────────────────────

const REVIEW_PROMPT = `You are reviewing a Claude Code skill. Your review has two parts: a quality review and a security review.

## Part 1: Install and Test

1. Install the skill by running: npx @gathers/skills add {skill_id}
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

// ── Helpers ─────────────────────────────────────────────────────────

const DIM = "\x1b[2m";
const BOLD = "\x1b[1m";
const GREEN = "\x1b[32m";
const CYAN = "\x1b[36m";
const YELLOW = "\x1b[33m";
const RED = "\x1b[31m";
const RESET = "\x1b[0m";

function log(msg) { console.log(msg); }
function dim(msg) { return `${DIM}${msg}${RESET}`; }
function bold(msg) { return `${BOLD}${msg}${RESET}`; }
function green(msg) { return `${GREEN}${msg}${RESET}`; }
function cyan(msg) { return `${CYAN}${msg}${RESET}`; }
function yellow(msg) { return `${YELLOW}${msg}${RESET}`; }
function red(msg) { return `${RED}${msg}${RESET}`; }

function makeTempDir() {
  const dir = join(tmpdir(), `gather-skills-${randomBytes(4).toString("hex")}`);
  mkdirSync(dir, { recursive: true });
  return dir;
}

function cleanupTemp(dir) {
  try { rmSync(dir, { recursive: true, force: true }); } catch {}
}

/** Detect which agents are present on this machine / in this project */
function detectAgents(isGlobal) {
  const found = [];
  for (const [name, paths] of Object.entries(AGENTS)) {
    if (isGlobal) {
      if (paths.global) found.push(name);
    } else {
      // Check if agent config dir exists in project or home
      const projectDir = join(process.cwd(), paths.dir.split("/")[0]);
      const homeDir = join(homedir(), paths.dir.split("/")[0]);
      if (existsSync(projectDir) || existsSync(homeDir)) {
        found.push(name);
      }
    }
  }
  // Default to claude-code if nothing detected
  if (found.length === 0) found.push("claude-code");
  return found;
}

/** Copy a skill directory, excluding .git, README.md, metadata.json, _-prefixed */
function copySkillDir(src, dest) {
  mkdirSync(dest, { recursive: true });
  const entries = readdirSync(src, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.name === ".git" || entry.name === "README.md" || entry.name === "metadata.json") continue;
    if (entry.name.startsWith("_")) continue;
    const srcPath = join(src, entry.name);
    const destPath = join(dest, entry.name);
    if (entry.isDirectory()) {
      copySkillDir(srcPath, destPath);
    } else {
      cpSync(srcPath, destPath);
    }
  }
}

/** Track install on our API (fire-and-forget) */
function trackInstall(skillId) {
  try {
    fetch(`${API}/api/skills/${skillId}/installed`, { method: "POST" }).catch(() => {});
  } catch {}
}

/** Parse skill ID: "owner/repo/skill" → { owner, repo, skill } */
function parseSkillId(id) {
  const parts = id.split("/");
  if (parts.length === 3) return { owner: parts[0], repo: parts[1], skill: parts[2] };
  if (parts.length === 2) return { owner: parts[0], repo: parts[1], skill: null };
  return null;
}

// ── Crypto / Attestation ────────────────────────────────────────────

function hashContent(content) {
  return createHash("sha256").update(content).digest("hex");
}

function getOrCreateKeyPair() {
  if (existsSync(KEYS_PATH)) {
    return JSON.parse(readFileSync(KEYS_PATH, "utf-8"));
  }

  mkdirSync(GATHER_DIR, { recursive: true });

  const { publicKey, privateKey } = generateKeyPairSync("ed25519", {
    publicKeyEncoding: { type: "spki", format: "pem" },
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
  });

  const keyPair = { publicKey, privateKey, createdAt: new Date().toISOString() };
  writeFileSync(KEYS_PATH, JSON.stringify(keyPair, null, 2));
  log(`  ${dim("Generated new Ed25519 keypair at")} ${dim(KEYS_PATH)}`);
  return keyPair;
}

function createAttestation(data) {
  const keyPair = getOrCreateKeyPair();
  const timestamp = Date.now();

  const payload = {
    skill_id: data.skill_id,
    task_hash: hashContent(data.task),
    output_hash: hashContent(data.cli_output || ""),
    score: data.score,
    security_score: data.security_score,
    timestamp,
  };

  const executionHash = hashContent(JSON.stringify({
    ...payload,
    what_worked: data.what_worked,
    what_failed: data.what_failed,
    execution_time_ms: data.execution_time_ms,
  }));

  const signature = sign(
    null,
    Buffer.from(executionHash),
    keyPair.privateKey,
  ).toString("base64");

  return {
    id: randomUUID(),
    version: "1.0.0",
    execution_hash: executionHash,
    payload,
    signature,
    public_key: keyPair.publicKey,
  };
}

// ── Executor ────────────────────────────────────────────────────────

function runClaude(prompt, cwd, verbose = false) {
  return new Promise((resolve) => {
    const stdoutChunks = [];
    const stderrChunks = [];
    let resolved = false;

    if (verbose) {
      log(`  ${dim("  [prompt length: " + prompt.length + " chars]")}`);
      log(`  ${dim("  [cwd: " + cwd + "]")}`);
    }

    // Pass prompt as argument instead of stdin — more reliable across platforms
    const proc = spawn("claude", ["--dangerously-skip-permissions", "-p", prompt], {
      cwd,
      stdio: ["ignore", "pipe", "pipe"],
      env: { ...process.env },
    });

    log(`  ${dim("  [spawned claude pid=" + proc.pid + "]")}`);

    const timer = setTimeout(() => {
      if (resolved) return;
      resolved = true;
      const partial = Buffer.concat(stdoutChunks).toString("utf-8");
      try { proc.kill("SIGTERM"); } catch {}
      setTimeout(() => { try { proc.kill("SIGKILL"); } catch {} }, 5000);
      resolve({
        output: partial + `\n\n[TIMEOUT] Review killed after ${Math.round(REVIEW_TIMEOUT_MS / 1000)}s`,
        exitCode: 1,
        timedOut: true,
      });
    }, REVIEW_TIMEOUT_MS);

    proc.stdout.on("data", (data) => {
      process.stdout.write(data);
      stdoutChunks.push(data);
    });
    proc.stderr.on("data", (data) => {
      if (verbose) process.stderr.write(data);
      stderrChunks.push(data);
    });

    proc.on("close", (code) => {
      clearTimeout(timer);
      if (resolved) return;
      resolved = true;
      const stdout = Buffer.concat(stdoutChunks).toString("utf-8");
      const stderr = Buffer.concat(stderrChunks).toString("utf-8");
      if (verbose || (!stdout.trim() && stderr.trim())) {
        log(`  ${dim("  [exit code: " + code + ", stdout: " + stdout.length + " bytes, stderr: " + stderr.length + " bytes]")}`);
        if (!stdout.trim() && stderr.trim()) {
          log(`  ${yellow("  stderr:")} ${stderr.slice(0, 500)}`);
        }
      }
      resolve({ output: stdout, exitCode: code ?? 1 });
    });

    proc.on("error", (err) => {
      clearTimeout(timer);
      if (resolved) return;
      resolved = true;
      resolve({ output: `Failed to spawn claude: ${err.message}`, exitCode: 1 });
    });
  });
}

function parseReviewResult(output) {
  const jsonMatch = output.match(/\{[\s\S]*"score"[\s\S]*"what_worked"[\s\S]*"what_failed"[\s\S]*"skill_feedback"[\s\S]*\}/);
  if (!jsonMatch) return null;

  try {
    const parsed = JSON.parse(jsonMatch[0]);
    if (
      typeof parsed.score === "number" &&
      typeof parsed.what_worked === "string" &&
      typeof parsed.what_failed === "string" &&
      typeof parsed.skill_feedback === "string"
    ) {
      return {
        score: Math.min(10, Math.max(1, parsed.score)),
        what_worked: parsed.what_worked,
        what_failed: parsed.what_failed,
        skill_feedback: parsed.skill_feedback,
        security_score: typeof parsed.security_score === "number" ? Math.min(10, Math.max(1, parsed.security_score)) : null,
        security_notes: typeof parsed.security_notes === "string" ? parsed.security_notes : null,
      };
    }
  } catch {}

  return null;
}

// ── Artifact Collection ──────────────────────────────────────────────

const ARTIFACT_EXTENSIONS = new Set([
  ".pptx", ".pdf", ".docx", ".xlsx",
  ".html", ".css", ".js", ".ts",
  ".json", ".xml", ".yaml", ".yml",
  ".png", ".jpg", ".jpeg", ".gif", ".svg",
  ".md", ".txt",
  ".zip", ".tar", ".gz",
]);

const MIME_TYPES = {
  ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
  ".pdf": "application/pdf",
  ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  ".html": "text/html", ".css": "text/css", ".js": "application/javascript",
  ".ts": "application/typescript", ".json": "application/json",
  ".xml": "application/xml", ".yaml": "application/yaml", ".yml": "application/yaml",
  ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
  ".gif": "image/gif", ".svg": "image/svg+xml",
  ".md": "text/markdown", ".txt": "text/plain",
  ".zip": "application/zip", ".tar": "application/x-tar", ".gz": "application/gzip",
};

const SKIP_DIRS = new Set(["node_modules", ".git", ".claude", "__pycache__", ".next", ".cache", ".venv", "venv"]);
const MAX_TOTAL_BYTES = 50 * 1024 * 1024;
const MAX_FILE_BYTES = 10 * 1024 * 1024;
const MAX_FILE_COUNT = 200;

function collectArtifacts(workDir) {
  const artifacts = [];
  let totalBytes = 0;

  function walk(dir) {
    if (artifacts.length >= MAX_FILE_COUNT || totalBytes >= MAX_TOTAL_BYTES) return;
    let entries;
    try { entries = readdirSync(dir, { withFileTypes: true }); } catch { return; }

    for (const entry of entries) {
      if (artifacts.length >= MAX_FILE_COUNT || totalBytes >= MAX_TOTAL_BYTES) return;

      if (entry.isDirectory()) {
        if (SKIP_DIRS.has(entry.name)) continue;
        walk(join(dir, entry.name));
        continue;
      }
      if (!entry.isFile()) continue;

      const ext = entry.name.includes(".") ? "." + entry.name.split(".").pop().toLowerCase() : "";
      if (!ARTIFACT_EXTENSIONS.has(ext)) continue;

      const filePath = join(dir, entry.name);
      let content;
      try { content = readFileSync(filePath); } catch { continue; }

      if (content.length > MAX_FILE_BYTES) continue;
      if (totalBytes + content.length > MAX_TOTAL_BYTES) continue;

      totalBytes += content.length;
      // Relative path from workDir
      const relPath = filePath.slice(workDir.length + 1);

      artifacts.push({
        file_name: relPath,
        mime_type: MIME_TYPES[ext] || "application/octet-stream",
        content,
      });
    }
  }

  try { walk(workDir); } catch {}
  return artifacts;
}

// ── Commands ────────────────────────────────────────────────────────

async function cmdInstall(flags) {
  log("");
  log(`${bold("skills.gather")} ${dim("— installing the Gather skill")}`);
  log("");

  const isGlobal = flags.global;
  const agents = detectAgents(isGlobal);

  // Fetch our SKILL.md directly from the API
  log(`  ${dim("→")} Fetching Gather skill...`);
  let skillContent;
  try {
    const res = await fetch(`${API}/api/skill`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    skillContent = await res.text();
  } catch (err) {
    log(`  ${red("✗")} Failed to fetch Gather skill: ${err.message}`);
    process.exit(1);
  }

  // Install to each detected agent
  for (const agent of agents) {
    const paths = AGENTS[agent];
    const base = isGlobal ? paths.global : join(process.cwd(), paths.dir);
    if (!base) continue;

    const skillDir = join(base, "gather");
    mkdirSync(skillDir, { recursive: true });
    writeFileSync(join(skillDir, "SKILL.md"), skillContent, "utf-8");
    log(`  ${green("✓")} Installed to ${dim(skillDir)}`);
  }

  log("");
  log(`  ${green("Done!")} The Gather skill is now active.`);
  log(`  ${dim("It will guide you through browsing, installing, and reviewing skills.")}`);
  log("");
}

async function cmdAdd(skillId, flags) {
  log("");
  log(`${bold("skills.gather")} ${dim("— installing")} ${cyan(skillId)}`);
  log("");

  const parsed = parseSkillId(skillId);
  if (!parsed) {
    log(`  ${red("✗")} Invalid skill ID. Use: ${yellow("owner/repo/skill-name")}`);
    log(`  ${dim("Example:")} npx @gathers/skills add anthropics/skills/pdf`);
    process.exit(1);
  }

  const isGlobal = flags.global;
  const agents = detectAgents(isGlobal);

  // Git clone the repo (shallow)
  const tmp = makeTempDir();
  const repoUrl = `https://github.com/${parsed.owner}/${parsed.repo}.git`;
  log(`  ${dim("→")} Cloning ${dim(parsed.owner + "/" + parsed.repo)}...`);
  try {
    execSync(`git clone --depth 1 --quiet "${repoUrl}" "${tmp}/repo"`, { stdio: "pipe" });
  } catch (err) {
    log(`  ${red("✗")} Failed to clone: ${err.message}`);
    cleanupTemp(tmp);
    process.exit(1);
  }

  // Find the skill directory — search multiple locations since repos
  // nest skills in various ways (root, skills/, skill-name/, skills/skill-name/)
  const repoDir = join(tmp, "repo");
  let skillDir = null;

  if (parsed.skill) {
    // 3-part ID: search for skill-name/SKILL.md at various depths
    const candidates = [
      join(repoDir, parsed.skill),                    // repo/pdf/
      join(repoDir, "skills", parsed.skill),           // repo/skills/pdf/
      join(repoDir, "src", parsed.skill),              // repo/src/pdf/
      repoDir,                                         // repo/ (root, single-skill repo)
    ];
    for (const candidate of candidates) {
      if (existsSync(join(candidate, "SKILL.md"))) {
        skillDir = candidate;
        break;
      }
    }
  } else {
    // 2-part ID: look for SKILL.md at root
    if (existsSync(join(repoDir, "SKILL.md"))) {
      skillDir = repoDir;
    }
  }

  if (!skillDir) {
    log(`  ${red("✗")} No SKILL.md found for "${parsed.skill || parsed.repo}"`);
    log(`  ${dim("Try checking the repo structure or skill name.")}`);
    cleanupTemp(tmp);
    process.exit(1);
  }

  // Check what files are in the skill
  const files = readdirSync(skillDir).filter(f => !f.startsWith(".") && !f.startsWith("_") && f !== "README.md" && f !== "metadata.json");
  log(`  ${green("✓")} Found skill: ${files.length} file(s)`);

  // Install to each detected agent
  const skillName = parsed.skill || parsed.repo;
  for (const agent of agents) {
    const paths = AGENTS[agent];
    const base = isGlobal ? paths.global : join(process.cwd(), paths.dir);
    if (!base) continue;

    const destDir = join(base, skillName);
    copySkillDir(skillDir, destDir);
    log(`  ${green("✓")} Installed to ${dim(agent)}: ${dim(destDir)}`);
  }

  // Track the install
  trackInstall(skillId);

  cleanupTemp(tmp);

  log("");
  log(`  ${green("Done!")} ${cyan(skillName)} is now installed.`);
  log(`  ${dim("Review it:")} npx @gathers/skills review ${skillId}`);
  log("");
}

async function pickRandomSkill() {
  try {
    const res = await fetch(`${API}/api/skills?limit=50&sort=newest`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    const skills = data.skills || data;
    if (!skills || skills.length === 0) throw new Error("No skills in catalog");
    const skill = skills[Math.floor(Math.random() * skills.length)];
    return skill.id;
  } catch (err) {
    log(`  ${red("✗")} Could not fetch skills for auto-pick: ${err.message}`);
    process.exit(1);
  }
}

async function cmdReview(skillId, flags) {
  log("");

  let apiKey = process.env.GATHER_API_KEY;
  if (!apiKey) {
    // Auto-register: frictionless onboarding
    const configPath = join(GATHER_DIR, "config.json");
    if (existsSync(configPath)) {
      try {
        const config = JSON.parse(readFileSync(configPath, "utf-8"));
        apiKey = config.api_key;
      } catch {}
    }
    if (!apiKey) {
      log(`  ${dim("→")} No API key found. Auto-registering...`);
      try {
        const hostname = execSync("hostname", { encoding: "utf-8" }).trim();
        const agentName = `agent-${hostname}-${randomBytes(3).toString("hex")}`;
        const res = await fetch(`${API}/api/auth/register`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name: agentName, description: "Auto-registered by npx @gathers/skills" }),
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        apiKey = data.api_key;
        mkdirSync(GATHER_DIR, { recursive: true });
        writeFileSync(configPath, JSON.stringify({ api_key: apiKey, agent_id: data.agent.id, agent_name: data.agent.name }, null, 2));
        log(`  ${green("✓")} Registered as ${cyan(data.agent.name)}`);
        log(`  ${dim("  Key saved to")} ${dim(configPath)}`);
      } catch (err) {
        log(`  ${red("✗")} Auto-registration failed: ${err.message}`);
        log(`  ${dim("Set GATHER_API_KEY manually or check network connectivity.")}`);
        process.exit(1);
      }
    }
  }

  // Auto-pick a random skill if none specified
  if (!skillId) {
    log(`${bold("skills.gather")} ${dim("— picking a random skill to review...")}`);
    log("");
    skillId = await pickRandomSkill();
    log(`  ${green("✓")} Selected: ${cyan(skillId)}`);
    log("");
  } else {
    log(`${bold("skills.gather")} ${dim("— reviewing")} ${cyan(skillId)}`);
    log("");
  }

  const task = flags.task || `Test the ${skillId} skill by performing a typical use case. Evaluate its functionality, documentation, ease of use, and security.`;

  log(`  ${dim("Skill:")} ${skillId}`);
  log(`  ${dim("Task:")} ${task.slice(0, 100)}${task.length > 100 ? "..." : ""}`);
  log("");

  // Build the full prompt
  const fullPrompt = REVIEW_PROMPT.replace("{skill_id}", skillId) + `\n\nSkill to review: ${skillId}\nTask: ${task}\n\nBegin by installing the skill and then executing the task.`;

  // Execute in a temp directory
  const workDir = makeTempDir();
  const startTime = Date.now();

  log(`  ${dim("─".repeat(60))}`);
  log(`  ${dim("Claude output:")}`);
  log("");

  const { output, exitCode, timedOut } = await runClaude(fullPrompt, workDir, flags.verbose);
  const executionTime = Date.now() - startTime;

  log("");
  log(`  ${dim("─".repeat(60))}`);

  // Collect artifacts BEFORE cleanup
  const artifacts = collectArtifacts(workDir);
  if (artifacts.length > 0) {
    log(`  ${green("✓")} Collected ${artifacts.length} artifact(s)`);
  }

  cleanupTemp(workDir);

  if (timedOut) {
    log(`  ${red("✗")} Review timed out after ${Math.round(REVIEW_TIMEOUT_MS / 1000)}s`);
    process.exit(1);
  }

  // Parse results
  const result = parseReviewResult(output);
  if (!result) {
    log(`  ${red("✗")} Failed to parse review output. Claude didn't return valid JSON.`);
    if (!output.trim()) {
      log(`  ${yellow("  Claude returned empty output.")}`);
      log(`  ${dim("  This usually means claude isn't installed or the prompt was rejected.")}`);
    } else {
      log(`  ${dim("  Output (last 300 chars):")} ${output.slice(-300)}`);
    }
    process.exit(1);
  }

  log(`  ${green("✓")} Review complete`);
  log("");
  log(`  ${cyan("Score:")}    ${result.score}/10`);
  log(`  ${cyan("Security:")} ${result.security_score !== null ? result.security_score + "/10" : "N/A"}`);
  if (result.security_notes) {
    log(`  ${cyan("Sec notes:")} ${result.security_notes}`);
  }
  log(`  ${cyan("Worked:")}  ${result.what_worked}`);
  log(`  ${cyan("Failed:")}  ${result.what_failed}`);
  log(`  ${cyan("Feedback:")} ${result.skill_feedback}`);
  log(`  ${dim(`Execution time: ${(executionTime / 1000).toFixed(1)}s`)}`);
  log("");

  // Generate attestation from the deterministic execution
  const attestation = createAttestation({
    skill_id: skillId,
    task,
    cli_output: output,
    score: result.score,
    security_score: result.security_score,
    what_worked: result.what_worked,
    what_failed: result.what_failed,
    execution_time_ms: executionTime,
  });

  log(`  ${green("✓")} Proof generated`);
  log(`  ${dim("  ID:")} ${attestation.id}`);
  log(`  ${dim("  Hash:")} ${attestation.execution_hash.slice(0, 16)}...`);
  log(`  ${dim("  Signature:")} ${attestation.signature.slice(0, 24)}...`);
  log("");

  // Submit to API
  log(`  ${dim("→")} Submitting to ${API}...`);

  try {
    // Ensure skill exists
    await fetch(`${API}/api/skills`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Api-Key": apiKey },
      body: JSON.stringify({ id: skillId, name: skillId.split("/").pop() || skillId }),
    });

    // Base64-encode artifacts for upload
    const artifactPayload = artifacts.map(a => ({
      file_name: a.file_name,
      mime_type: a.mime_type,
      content_base64: a.content.toString("base64"),
    }));

    const response = await fetch(`${API}/api/reviews/submit`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Api-Key": apiKey },
      body: JSON.stringify({
        skill_id: skillId,
        task,
        score: result.score,
        what_worked: result.what_worked,
        what_failed: result.what_failed,
        skill_feedback: result.skill_feedback,
        security_score: result.security_score,
        security_notes: result.security_notes,
        runner_type: "claude",
        execution_time_ms: executionTime,
        cli_output: output,
        proof: {
          id: attestation.id,
          signature: attestation.signature,
          public_key: attestation.public_key,
          execution_hash: attestation.execution_hash,
          payload: attestation.payload,
        },
        artifacts: artifactPayload,
      }),
    });

    if (response.ok) {
      const data = await response.json();
      log(`  ${green("✓")} Review submitted with proof`);
      if (data.artifact_count > 0) log(`  ${green("✓")} ${data.artifact_count} artifact(s) uploaded`);
      log(`  ${dim("  Review:")} ${API}/skills/${encodeURIComponent(skillId)}`);
      if (data.review_id) log(`  ${dim("  ID:")} ${data.review_id}`);
      if (data.proof_id) log(`  ${dim("  Proof:")} ${data.proof_id}`);
    } else {
      const error = await response.text();
      log(`  ${red("✗")} Submit failed: ${error}`);
    }
  } catch (err) {
    log(`  ${red("✗")} Could not reach API: ${err.message}`);
  }

  log("");
}

async function cmdBrowse(category) {
  log("");
  log(`${bold("skills.gather")} ${dim("— browsing skills")}`);
  log("");

  const url = category ? `${API}/c/${category}` : `${API}/llms.txt`;

  try {
    const res = await fetch(url);
    if (!res.ok) {
      if (res.status === 404) {
        log(`  ${red("✗")} Category "${category}" not found.`);
        log(`  ${dim("Try:")} npx @gathers/skills browse frontend`);
      } else {
        log(`  ${red("✗")} HTTP ${res.status}`);
      }
      process.exit(1);
    }
    const text = await res.text();
    log(text);
  } catch (err) {
    log(`  ${red("✗")} Failed to fetch: ${err.message}`);
    process.exit(1);
  }
}

function cmdHelp() {
  log("");
  log(`${bold("skills.gather")} ${dim("— browse, install, review AI agent skills with cryptographic proofs")}`);
  log("");
  log(`  ${cyan("npx @gathers/skills install")}              Install the Gather skill (do this first)`);
  log(`  ${cyan("npx @gathers/skills add")} ${yellow("<owner/repo/skill>")}  Install a skill from the catalog`);
  log(`  ${cyan("npx @gathers/skills review")} ${dim("[skill_id]")}        Review a skill (auto-picks if omitted)`);
  log(`  ${cyan("npx @gathers/skills browse")} ${dim("[category]")}     Browse skills (shows /llms.txt or category)`);
  log(`  ${cyan("npx @gathers/skills help")}                 Show this help`);
  log("");
  log(`  ${dim("Flags:")}`);
  log(`    ${dim("-g, --global")}    Install globally (all projects)`);
  log(`    ${dim("-t, --task")}      Task for the review (review command only)`);
  log("");
  log(`  ${dim("Environment:")}`);
  log(`    ${dim("GATHER_API_KEY")}  API key for submitting reviews (from /api/auth/register)`);
  log("");
  log(`  ${dim("Examples:")}`);
  log(`    npx @gathers/skills install`);
  log(`    npx @gathers/skills add anthropics/skills/pdf`);
  log(`    npx @gathers/skills review anthropics/skills/pdf`);
  log(`    npx @gathers/skills review anthropics/skills/pdf -t "Generate a report from markdown"`);
  log(`    npx @gathers/skills browse frontend`);
  log("");
  log(`  ${dim(`${API}/llms.txt`)}`);
  log("");
}

// ── Main ────────────────────────────────────────────────────────────

const args = process.argv.slice(2);
const command = args[0];
const flags = {
  global: args.includes("-g") || args.includes("--global"),
  verbose: args.includes("-v") || args.includes("--verbose"),
};

// Parse --task / -t flag
const taskIdx = args.findIndex(a => a === "-t" || a === "--task");
if (taskIdx !== -1 && args[taskIdx + 1]) {
  flags.task = args[taskIdx + 1];
}

// Filter out flags from positional args
const positional = args.filter((a, i) => !a.startsWith("-") && (i === 0 || (args[i - 1] !== "-t" && args[i - 1] !== "--task")));

switch (command) {
  case "install":
    await cmdInstall(flags);
    break;
  case "add":
    if (!positional[1]) {
      log(`\n  ${red("✗")} Missing skill ID.`);
      log(`  ${dim("Usage:")} npx @gathers/skills add ${yellow("owner/repo/skill-name")}\n`);
      process.exit(1);
    }
    await cmdAdd(positional[1], flags);
    break;
  case "review":
    await cmdReview(positional[1] || null, flags);
    break;
  case "browse":
    await cmdBrowse(positional[1]);
    break;
  case "help":
  case "--help":
  case "-h":
    cmdHelp();
    break;
  default:
    cmdHelp();
    break;
}
