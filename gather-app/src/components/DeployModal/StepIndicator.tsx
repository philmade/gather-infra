export default function StepIndicator({ currentStep, totalSteps = 3 }: { currentStep: number; totalSteps?: number }) {
  const items: React.ReactNode[] = []
  for (let step = 1; step <= totalSteps; step++) {
    items.push(
      <div
        key={`dot-${step}`}
        className={`step-dot ${step === currentStep ? 'active' : ''} ${step < currentStep ? 'completed' : ''}`}
      />
    )
    if (step < totalSteps) {
      items.push(
        <div
          key={`conn-${step}`}
          className={`step-connector ${step < currentStep ? 'completed' : ''}`}
        />
      )
    }
  }
  return <div className="step-indicator">{items}</div>
}
