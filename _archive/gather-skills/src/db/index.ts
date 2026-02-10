import Database from 'better-sqlite3';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { existsSync, mkdirSync } from 'fs';
import { SCHEMA } from './schema.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

let db: Database.Database | null = null;

export function getDbPath(): string {
  return process.env.DATABASE_PATH || join(__dirname, '../../data/reskill.db');
}

export function getDb(): Database.Database {
  if (db) return db;

  const dbPath = getDbPath();
  const dbDir = dirname(dbPath);

  if (!existsSync(dbDir)) {
    mkdirSync(dbDir, { recursive: true });
  }

  db = new Database(dbPath);
  db.pragma('journal_mode = WAL');
  db.pragma('foreign_keys = ON');

  return db;
}

export function initDb(): void {
  const database = getDb();
  database.exec(SCHEMA);

  // Run migrations for existing databases
  runMigrations(database);
}

function runMigrations(database: Database.Database): void {
  // Check and add runner_type column
  const reviewColumns = database.prepare("PRAGMA table_info(reviews)").all() as Array<{ name: string }>;
  const columnNames = reviewColumns.map(c => c.name);

  if (!columnNames.includes('runner_type')) {
    database.exec("ALTER TABLE reviews ADD COLUMN runner_type TEXT DEFAULT 'claude'");
    console.log('Migration: Added runner_type column to reviews');
  }

  if (!columnNames.includes('permission_mode')) {
    database.exec("ALTER TABLE reviews ADD COLUMN permission_mode TEXT DEFAULT 'default'");
    console.log('Migration: Added permission_mode column to reviews');
  }

  // Check and add category column to skills
  const skillColumns = database.prepare("PRAGMA table_info(skills)").all() as Array<{ name: string }>;
  const skillColumnNames = skillColumns.map(c => c.name);

  if (!skillColumnNames.includes('category')) {
    database.exec("ALTER TABLE skills ADD COLUMN category TEXT DEFAULT NULL");
    console.log('Migration: Added category column to skills');
  }

  // Re-read review columns for security fields
  const reviewCols2 = database.prepare("PRAGMA table_info(reviews)").all() as Array<{ name: string }>;
  const reviewColNames2 = reviewCols2.map(c => c.name);

  if (!reviewColNames2.includes('security_score')) {
    database.exec("ALTER TABLE reviews ADD COLUMN security_score REAL DEFAULT NULL");
    console.log('Migration: Added security_score column to reviews');
  }

  if (!reviewColNames2.includes('security_notes')) {
    database.exec("ALTER TABLE reviews ADD COLUMN security_notes TEXT DEFAULT NULL");
    console.log('Migration: Added security_notes column to reviews');
  }

  // Add avg_security_score to skills
  const skillCols2 = database.prepare("PRAGMA table_info(skills)").all() as Array<{ name: string }>;
  const skillColNames2 = skillCols2.map(c => c.name);

  if (!skillColNames2.includes('avg_security_score')) {
    database.exec("ALTER TABLE skills ADD COLUMN avg_security_score REAL DEFAULT NULL");
    console.log('Migration: Added avg_security_score column to skills');
  }

  // Add indexes for search performance
  const indexes = database.prepare("SELECT name FROM sqlite_master WHERE type = 'index'").all() as Array<{ name: string }>;
  const indexNames = indexes.map(i => i.name);

  if (!indexNames.includes('idx_skills_category')) {
    database.exec("CREATE INDEX idx_skills_category ON skills(category)");
    console.log('Migration: Added idx_skills_category index');
  }

  if (!indexNames.includes('idx_skills_name')) {
    database.exec("CREATE INDEX idx_skills_name ON skills(name)");
    console.log('Migration: Added idx_skills_name index');
  }

  // Add file_path and size_bytes columns to artifacts
  const artifactColumns = database.prepare("PRAGMA table_info(artifacts)").all() as Array<{ name: string }>;
  const artifactColNames = artifactColumns.map(c => c.name);

  if (!artifactColNames.includes('file_path')) {
    database.exec("ALTER TABLE artifacts ADD COLUMN file_path TEXT");
    console.log('Migration: Added file_path column to artifacts');
  }

  if (!artifactColNames.includes('size_bytes')) {
    database.exec("ALTER TABLE artifacts ADD COLUMN size_bytes INTEGER");
    console.log('Migration: Added size_bytes column to artifacts');
  }
}

export function closeDb(): void {
  if (db) {
    db.close();
    db = null;
  }
}

export { Database };
