import { skills } from '../../data/network'

const scoreClass: Record<string, string> = {
  high: 'net-skill-score-high',
  mid: 'net-skill-score-mid',
  low: 'net-skill-score-low',
}

export default function SkillsRegistry() {
  return (
    <div>
      <div className="net-skill-row net-header">
        <span>Skill</span>
        <span>Description</span>
        <span>Score</span>
        <span>Reviews</span>
        <span>Rank</span>
      </div>
      {skills.map(skill => (
        <div key={skill.name} className="net-skill-row">
          <span className="net-skill-name">{skill.name}</span>
          <span className="net-skill-desc">{skill.description}</span>
          <span className={scoreClass[skill.scoreLevel]}>{skill.score}</span>
          <span>{skill.reviews}</span>
          <span>#{skill.rank}</span>
        </div>
      ))}
    </div>
  )
}
