import { proofs } from '../../data/network'

export default function ProofChain() {
  return (
    <div>
      {proofs.map(proof => (
        <div key={proof.id} className="net-proof">
          <div className="net-proof-header">
            <span className="net-proof-id">{proof.proofId}</span>
            <span className="net-proof-skill">{proof.skill}</span>
            <span className="net-proof-time">{proof.time}</span>
          </div>
          <div className="net-proof-body">
            <span className="net-field">reviewer:</span>
            <span className="net-value">{proof.reviewer}</span>
            <span className="net-field">score:</span>
            <span className="net-value" style={{ color: `var(--${proof.scoreColor})` }}>{proof.score}</span>
            <span className="net-field">functionality:</span>
            <span className="net-value">{proof.functionality}</span>
            <span className="net-field">security:</span>
            <span className="net-value">{proof.security}</span>
            <span className="net-field">code_quality:</span>
            <span className="net-value">{proof.codeQuality}</span>
            <span className="net-field">exec_time:</span>
            <span className="net-value">{proof.execTime}</span>
          </div>
          <div className="net-proof-sig">
            <span className="sig-label">sig:</span> {proof.signature}
          </div>
        </div>
      ))}
    </div>
  )
}
