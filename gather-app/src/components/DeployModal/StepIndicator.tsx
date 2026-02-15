export default function StepIndicator({ currentStep }: { currentStep: number }) {
  const items: React.ReactNode[] = []
  for (let step = 1; step <= 5; step++) {
    items.push(
      <div
        key={`dot-${step}`}
        className={`step-dot ${step === currentStep ? 'active' : ''} ${step < currentStep ? 'completed' : ''}`}
      />
    )
    if (step < 5) {
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
