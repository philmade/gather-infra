import { Command } from 'commander';
import chalk from 'chalk';
import { getDb } from '../../db/index.js';

const command = new Command('status')
  .description('Check the status of a review')
  .argument('<review_id>', 'Review ID to check')
  .option('--output', 'Show full CLI output', false)
  .action((reviewId: string, options: { output: boolean }) => {
    const db = getDb();

    const review = db.prepare(`
      SELECT r.*, s.name as skill_name
      FROM reviews r
      JOIN skills s ON r.skill_id = s.id
      WHERE r.id = ?
    `).get(reviewId) as {
      id: string;
      skill_id: string;
      skill_name: string;
      task: string;
      status: string;
      score: number | null;
      what_worked: string | null;
      what_failed: string | null;
      skill_feedback: string | null;
      agent_model: string | null;
      execution_time_ms: number | null;
      cli_output: string | null;
      proof_id: string | null;
      created_at: string;
    } | undefined;

    if (!review) {
      console.log(chalk.red('Review not found'));
      process.exit(1);
    }

    // Status color
    const statusColors: Record<string, (s: string) => string> = {
      pending: chalk.yellow,
      running: chalk.blue,
      complete: chalk.green,
      failed: chalk.red,
    };
    const statusColor = statusColors[review.status] || chalk.white;

    console.log(chalk.bold('Review Status'));
    console.log();
    console.log(chalk.dim('ID:'), review.id);
    console.log(chalk.dim('Skill:'), `${review.skill_name} (${review.skill_id})`);
    console.log(chalk.dim('Status:'), statusColor(review.status));
    console.log(chalk.dim('Created:'), review.created_at);
    console.log();
    console.log(chalk.dim('Task:'));
    console.log(review.task);
    console.log();

    if (review.status === 'complete') {
      console.log(chalk.bold('Results'));
      console.log();
      console.log(chalk.dim('Score:'), review.score !== null ? chalk.cyan(`${review.score}/10`) : 'N/A');
      console.log();
      console.log(chalk.dim('What worked:'));
      console.log(review.what_worked || 'N/A');
      console.log();
      console.log(chalk.dim('What failed:'));
      console.log(review.what_failed || 'N/A');
      console.log();
      console.log(chalk.dim('Skill feedback:'));
      console.log(review.skill_feedback || 'N/A');
      console.log();

      if (review.agent_model) {
        console.log(chalk.dim('Agent model:'), review.agent_model);
      }
      if (review.execution_time_ms) {
        console.log(chalk.dim('Execution time:'), `${(review.execution_time_ms / 1000).toFixed(1)}s`);
      }
      if (review.proof_id) {
        console.log(chalk.dim('Proof ID:'), review.proof_id);
      }
    }

    if (options.output && review.cli_output) {
      console.log();
      console.log(chalk.bold('CLI Output'));
      console.log(chalk.dim('─'.repeat(40)));
      console.log(review.cli_output);
    }
  });

export default command;
