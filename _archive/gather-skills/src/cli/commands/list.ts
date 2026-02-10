import { Command } from 'commander';
import chalk from 'chalk';
import { getDb } from '../../db/index.js';

const command = new Command('list')
  .description('List top-ranked skills')
  .option('-n, --limit <number>', 'Number of skills to show', '20')
  .option('--reviews', 'Show recent reviews instead of skills', false)
  .action((options: { limit: string; reviews: boolean }) => {
    const db = getDb();
    const limit = Math.min(100, parseInt(options.limit, 10) || 20);

    if (options.reviews) {
      // List recent reviews
      const reviews = db.prepare(`
        SELECT r.id, r.skill_id, r.status, r.score, r.created_at, s.name as skill_name
        FROM reviews r
        JOIN skills s ON r.skill_id = s.id
        ORDER BY r.created_at DESC
        LIMIT ?
      `).all(limit) as Array<{
        id: string;
        skill_id: string;
        skill_name: string;
        status: string;
        score: number | null;
        created_at: string;
      }>;

      if (reviews.length === 0) {
        console.log(chalk.dim('No reviews yet'));
        return;
      }

      console.log(chalk.bold('Recent Reviews'));
      console.log();

      for (const review of reviews) {
        const statusColors: Record<string, (s: string) => string> = {
          pending: chalk.yellow,
          running: chalk.blue,
          complete: chalk.green,
          failed: chalk.red,
        };
        const statusColor = statusColors[review.status] || chalk.white;
        const score = review.score !== null ? chalk.cyan(`${review.score}/10`) : chalk.dim('--');

        console.log(
          `${chalk.dim(review.id.slice(0, 8))}  ${statusColor(review.status.padEnd(8))}  ${score.padEnd(12)}  ${review.skill_name}`
        );
      }
    } else {
      // List skills by rank
      const skills = db.prepare(`
        SELECT id, name, review_count, avg_score, rank_score
        FROM skills
        ORDER BY rank_score DESC NULLS LAST, review_count DESC
        LIMIT ?
      `).all(limit) as Array<{
        id: string;
        name: string;
        review_count: number;
        avg_score: number | null;
        rank_score: number | null;
      }>;

      if (skills.length === 0) {
        console.log(chalk.dim('No skills yet. Start a review with:'));
        console.log(chalk.cyan('  npx @gathers/skills review <skill_id>'));
        return;
      }

      console.log(chalk.bold('Top Skills'));
      console.log();
      console.log(
        chalk.dim('Rank'.padEnd(6)) +
        chalk.dim('Score'.padEnd(8)) +
        chalk.dim('Reviews'.padEnd(10)) +
        chalk.dim('Name')
      );
      console.log(chalk.dim('─'.repeat(60)));

      skills.forEach((skill, i) => {
        const rank = `#${i + 1}`.padEnd(6);
        const avgScore = skill.avg_score !== null ? `${skill.avg_score.toFixed(1)}/10` : '--';
        const reviews = skill.review_count.toString();

        console.log(
          `${rank}${avgScore.padEnd(8)}${reviews.padEnd(10)}${skill.name}`
        );
      });

      console.log();
      console.log(chalk.dim(`Showing ${skills.length} skills`));
    }
  });

export default command;
