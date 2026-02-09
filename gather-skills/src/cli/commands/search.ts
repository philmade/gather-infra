import { Command } from 'commander';
import chalk from 'chalk';
import { getDb } from '../../db/index.js';

const command = new Command('search')
  .description('Search for skills')
  .argument('<query>', 'Search query')
  .option('-n, --limit <number>', 'Number of results', '10')
  .action((query: string, options: { limit: string }) => {
    const db = getDb();
    const limit = Math.min(50, parseInt(options.limit, 10) || 10);

    const skills = db.prepare(`
      SELECT id, name, description, review_count, avg_score
      FROM skills
      WHERE name LIKE ? OR description LIKE ? OR id LIKE ?
      ORDER BY rank_score DESC NULLS LAST, review_count DESC
      LIMIT ?
    `).all(`%${query}%`, `%${query}%`, `%${query}%`, limit) as Array<{
      id: string;
      name: string;
      description: string | null;
      review_count: number;
      avg_score: number | null;
    }>;

    if (skills.length === 0) {
      console.log(chalk.dim(`No skills found matching "${query}"`));
      return;
    }

    console.log(chalk.bold(`Search results for "${query}"`));
    console.log();

    for (const skill of skills) {
      const avgScore = skill.avg_score !== null ? chalk.cyan(`${skill.avg_score.toFixed(1)}/10`) : chalk.dim('--');
      const reviews = skill.review_count > 0 ? `${skill.review_count} reviews` : 'no reviews';

      console.log(chalk.white(skill.name));
      console.log(chalk.dim(`  ${skill.id}`));
      if (skill.description) {
        console.log(chalk.dim(`  ${skill.description.slice(0, 80)}${skill.description.length > 80 ? '...' : ''}`));
      }
      console.log(chalk.dim(`  Score: ${avgScore}  |  ${reviews}`));
      console.log();
    }
  });

export default command;
