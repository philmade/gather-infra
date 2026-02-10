import 'dotenv/config';
import { initDb, closeDb, getDbPath } from './index.js';

console.log('Initializing database at:', getDbPath());
initDb();
console.log('Database initialized successfully');
closeDb();
