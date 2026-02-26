// Resilient loop — a dual-agent loop with retry logic.
//
// The pattern: executor → reviewer → loop_control, repeated until the reviewer
// signals LOOP_DONE or LOOP_PAUSE. Each sub-agent is retried up to maxRetries
// times with exponential backoff on error. Escalation events from loop_control
// are swallowed (not propagated to the parent) so the orchestrator can continue
// after a loop finishes.
package core

import (
	"fmt"
	"iter"
	"log"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	adkmodel "google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

const maxRetries = 3

// newResilientLoop creates a loop agent that runs executor → reviewer → control
// in sequence, retrying sub-agents on error instead of killing the stream.
func newResilientLoop(name, description, reviewerStateKey string, maxIter uint, executor, reviewer agent.Agent) (agent.Agent, error) {
	controlName := name + "_control"

	loopControl, err := newLoopControl(controlName, reviewerStateKey)
	if err != nil {
		return nil, err
	}

	return agent.New(agent.Config{
		Name:        name,
		Description: description,
		SubAgents:   []agent.Agent{executor, reviewer, loopControl},
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				remaining := maxIter
				iteration := 0
				for {
					iteration++
					if maxIter > 0 {
						if remaining == 0 {
							log.Printf("%s: max iterations (%d) reached", name, maxIter)
							return
						}
						remaining--
					}

					log.Printf("%s: iteration %d", name, iteration)
					shouldExit := false

					for _, sub := range ctx.Agent().SubAgents() {
						success := false
						for attempt := 1; attempt <= maxRetries; attempt++ {
							errored := false
							for event, err := range sub.Run(ctx) {
								if err != nil {
									log.Printf("%s: %s error (attempt %d/%d): %v",
										name, sub.Name(), attempt, maxRetries, err)
									errored = true
									break
								}
								// Swallow escalation events — use them as a signal to
								// stop the loop but do NOT propagate Escalate to the parent.
								// If we yield Escalate=true, ADK terminates the parent
								// LLMAgent too, preventing the orchestrator from continuing.
								if event.Actions.Escalate {
									log.Printf("%s: escalation event from %s (swallowed, not propagated)", name, sub.Name())
									shouldExit = true
									continue
								}
								if !yield(event, nil) {
									return
								}
							}
							if !errored {
								success = true
								break
							}
							if attempt < maxRetries {
								backoff := time.Duration(attempt*2) * time.Second
								log.Printf("%s: retrying %s in %s", name, sub.Name(), backoff)
								time.Sleep(backoff)
							}
						}
						if !success {
							log.Printf("%s: %s failed after %d attempts, skipping",
								name, sub.Name(), maxRetries)
						}
						if shouldExit {
							log.Printf("%s: escalation received, exiting loop", name)
							return
						}
					}
				}
			}
		},
	})
}

// newLoopControl creates a custom agent that reads the reviewer's state key
// and escalates when LOOP_DONE or LOOP_PAUSE is detected.
func newLoopControl(name, reviewerStateKey string) (agent.Agent, error) {
	return agent.New(agent.Config{
		Name:        name,
		Description: "Reads reviewer output and escalates to end the loop when appropriate.",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				output, err := ctx.Session().State().Get(reviewerStateKey)
				if err != nil {
					return
				}
				text, ok := output.(string)
				if !ok {
					return
				}

				upper := strings.ToUpper(text)
				if strings.Contains(upper, "LOOP_DONE") || strings.Contains(upper, "LOOP_PAUSE") {
					signal := "LOOP_DONE"
					if strings.Contains(upper, "LOOP_PAUSE") {
						signal = "LOOP_PAUSE"
					}
					log.Printf("%s: escalating (%s)", name, signal)

					evt := session.NewEvent(ctx.InvocationID())
					evt.Author = name
					evt.Branch = ctx.Branch()
					evt.LLMResponse = adkmodel.LLMResponse{
						Content: &genai.Content{
							Role: genai.RoleModel,
							Parts: []*genai.Part{genai.NewPartFromText(
								fmt.Sprintf("Loop complete (%s). Returning to orchestrator.", signal),
							)},
						},
					}
					evt.Actions.Escalate = true
					yield(evt, nil)
				}
			}
		},
	})
}
