import { Command } from 'commander';
import chalk from 'chalk';
import { nanoid } from 'nanoid';
import { getDb } from '../../db/index.js';
import { executeReview, ExecuteResult } from '../../worker/executor.js';

const API_URL = process.env.API_URL || 'https://skills.gather.is';
const API_KEY = process.env.GATHER_API_KEY;

async function submitToRemote(result: ExecuteResult, skillId: string, task: string): Promise<void> {
  if (!API_KEY) {
    console.log(chalk.dim('No GATHER_API_KEY set - results stored locally only'));
    return;
  }

  try {
    // First ensure skill exists on remote
    await fetch(`${API_URL}/api/skills`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Api-Key': API_KEY,
      },
      body: JSON.stringify({
        id: skillId,
        name: skillId.split('/').pop() || skillId,
      }),
    });

    // Base64-encode artifacts for upload
    const artifacts = result.artifacts.map(a => ({
      file_name: a.file_name,
      mime_type: a.mime_type,
      content_base64: a.content.toString('base64'),
    }));

    // Submit the review with results
    const response = await fetch(`${API_URL}/api/reviews/submit`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Api-Key': API_KEY,
      },
      body: JSON.stringify({
        skill_id: skillId,
        task,
        score: result.score,
        what_worked: result.what_worked,
        what_failed: result.what_failed,
        skill_feedback: result.skill_feedback,
        security_score: result.security_score,
        security_notes: result.security_notes,
        runner_type: 'claude',
        permission_mode: result.cli_output.includes('--dangerously-skip-permissions') ? 'dangerous' : 'default',
        execution_time_ms: result.execution_time_ms,
        cli_output: result.cli_output,
        proof: result.proof,
        artifacts,
      }),
    });

    if (response.ok) {
      const data = await response.json() as { review_id?: string };
      console.log(chalk.green('✓ Review submitted to'), chalk.cyan(`${API_URL}/skills/${encodeURIComponent(skillId)}`));
      if (data.review_id) {
        console.log(chalk.dim(`Remote review ID: ${data.review_id}`));
      }
    } else {
      const error = await response.text();
      console.log(chalk.yellow('⚠️  Failed to submit to remote:'), error);
    }
  } catch (err) {
    console.log(chalk.yellow('⚠️  Could not reach remote API:'), err instanceof Error ? err.message : String(err));
  }
}

const command = new Command('review')
  .description('Start a review of a skill')
  .argument('<skill_id>', 'Skill ID to review (e.g., anthropics/skills/pptx)')
  .option('-t, --task <task>', 'Task for the agent to perform with the skill')
  .option('--wait', 'Wait for review to complete', false)
  .option('--dangerous', 'Skip all permission prompts (use with caution)', false)
  .option('--local', 'Store results locally only (do not submit to remote)', false)
  .action(async (skillId: string, options: { task?: string; wait: boolean; dangerous: boolean; local: boolean }) => {
    const db = getDb();

    // Default task if not provided
    const task = options.task || `Test the ${skillId} skill by performing a typical use case. Evaluate its functionality, documentation, and ease of use.`;

    // Check if skill exists, if not create it
    let skill = db.prepare('SELECT id, name FROM skills WHERE id = ?').get(skillId) as { id: string; name: string } | undefined;
    if (!skill) {
      const name = skillId.split('/').pop() || skillId;
      db.prepare(`
        INSERT INTO skills (id, name, source)
        VALUES (?, ?, 'github')
      `).run(skillId, name);
      skill = { id: skillId, name };
      console.log(chalk.dim(`Created skill: ${name}`));
    }

    // Create review
    const reviewId = nanoid();
    db.prepare(`
      INSERT INTO reviews (id, skill_id, task, status)
      VALUES (?, ?, ?, 'pending')
    `).run(reviewId, skillId, task);

    console.log(chalk.blue('Review ID:'), reviewId);
    console.log(chalk.blue('Skill:'), skill.name);
    console.log(chalk.blue('Task:'), task.slice(0, 100) + (task.length > 100 ? '...' : ''));
    console.log();

    if (options.dangerous) {
      console.log(chalk.yellow('⚠️  Running with --dangerous: all permission prompts will be skipped'));
      console.log();
    }

    if (options.wait) {
      console.log(chalk.dim('─'.repeat(60)));
      console.log(chalk.dim('Claude output:'));
      console.log();

      try {
        const result = await executeReview(reviewId, skillId, task, { stream: true, dangerous: options.dangerous });
        console.log();
        console.log(chalk.dim('─'.repeat(60)));

        if (result.status === 'complete') {
          console.log(chalk.green('✓ Review complete'));
          console.log();
          console.log(chalk.blue('Score:'), result.score !== undefined ? `${result.score}/10` : 'N/A');
          console.log(chalk.blue('Security:'), result.security_score !== undefined ? `${result.security_score}/10` : 'N/A');
          if (result.security_notes) {
            console.log(chalk.blue('Security notes:'), result.security_notes);
          }
          console.log(chalk.blue('What worked:'), result.what_worked || 'N/A');
          console.log(chalk.blue('What failed:'), result.what_failed || 'N/A');
          console.log(chalk.blue('Feedback:'), result.skill_feedback || 'N/A');
          console.log(chalk.dim(`Execution time: ${(result.execution_time_ms / 1000).toFixed(1)}s`));

          if (result.artifacts.length > 0) {
            console.log(chalk.blue('Artifacts:'), result.artifacts.map(a => a.file_name).join(', '));
          }

          // Submit to remote unless --local flag
          if (!options.local) {
            console.log();
            await submitToRemote(result, skillId, task);
          }
        } else {
          console.log(chalk.red('✗ Review failed'));
          console.log(chalk.dim('Check status for details: reskill status ' + reviewId));
        }
      } catch (error) {
        console.log(chalk.red('✗ Review failed'));
        console.error(chalk.red(error instanceof Error ? error.message : String(error)));
      }
    } else {
      // Start in background
      executeReview(reviewId, skillId, task, { dangerous: options.dangerous })
        .then(async (result) => {
          if (result.status === 'complete' && !options.local) {
            await submitToRemote(result, skillId, task);
          }
        })
        .catch((err) => {
          console.error(`Review ${reviewId} failed:`, err);
        });

      console.log(chalk.yellow('Review started in background'));
      console.log(chalk.dim(`Check status: reskill status ${reviewId}`));
    }
  });

export default command;
