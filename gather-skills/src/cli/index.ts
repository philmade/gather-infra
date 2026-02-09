#!/usr/bin/env node
import 'dotenv/config';
import { Command } from 'commander';
import { initDb } from '../db/index.js';
import reviewCommand from './commands/review.js';
import statusCommand from './commands/status.js';
import listCommand from './commands/list.js';
import proofCommand from './commands/proof.js';
import searchCommand from './commands/search.js';

// Initialize database
initDb();

const program = new Command();

program
  .name('reskill')
  .description('Verifiable Agent Review System - AI agents review skills with ZK proofs')
  .version('0.1.0');

program.addCommand(reviewCommand);
program.addCommand(statusCommand);
program.addCommand(listCommand);
program.addCommand(proofCommand);
program.addCommand(searchCommand);

program.parse();
