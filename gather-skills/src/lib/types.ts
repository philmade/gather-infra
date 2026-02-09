export interface Skill {
  id: string;
  name: string;
  description: string | null;
  source: 'skills.sh' | 'github';
  category: string | null;
  installs: number;
  review_count: number;
  avg_score: number | null;
  avg_security_score: number | null;
  rank_score: number | null;
  created_at: string;
}

export interface Review {
  id: string;
  skill_id: string;
  task: string;
  status: 'pending' | 'running' | 'complete' | 'failed';
  score: number | null;
  what_worked: string | null;
  what_failed: string | null;
  skill_feedback: string | null;
  security_score: number | null;
  security_notes: string | null;
  agent_model: string | null;
  execution_time_ms: number | null;
  cli_output: string | null;
  proof_id: string | null;
  created_at: string;
}

export interface Proof {
  id: string;
  review_id: string;
  claim_data: string;
  identifier: string;
  signatures: string;
  witnesses: string;
  verified: boolean;
  created_at: string;
}

export interface Artifact {
  id: string;
  review_id: string;
  file_name: string;
  mime_type: string | null;
  content: Buffer;
  created_at: string;
}

export interface ReviewResult {
  score: number;
  what_worked: string;
  what_failed: string;
  skill_feedback: string;
  security_score: number | null;
  security_notes: string | null;
  artifacts?: Array<{
    file_name: string;
    mime_type: string;
    content: Buffer;
  }>;
}

export interface ExecutionLogs {
  install_log: string;
  test_output: string;
  claude_response: string;
}

export interface RankingWeights {
  reviews: number;
  installs: number;
  proofs: number;
}
