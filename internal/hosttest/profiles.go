package hosttest

import (
	"context"

	"github.com/kdlbs/kandev/pluginsdk"
)

// AddWorkflow registers a workflow and its steps.
func (h *Host) AddWorkflow(workflow pluginsdk.Workflow, steps ...pluginsdk.WorkflowStep) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.steps == nil {
		h.steps = map[string][]pluginsdk.WorkflowStep{}
	}
	h.workflows = append(h.workflows, workflow)
	h.steps[workflow.ID] = steps
}

// AddAgentProfile registers an agent profile the plugin may launch with.
func (h *Host) AddAgentProfile(profile pluginsdk.AgentProfile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agentProfiles = append(h.agentProfiles, profile)
}

// AddExecutorProfile registers an executor profile the plugin may launch with.
func (h *Host) AddExecutorProfile(profile pluginsdk.ExecutorProfile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.executorProfiles = append(h.executorProfiles, profile)
}

func (h *Host) Workflows() pluginsdk.WorkflowReader               { return workflowReader{h} }
func (h *Host) AgentProfiles() pluginsdk.AgentProfileReader       { return agentProfileReader{h} }
func (h *Host) ExecutorProfiles() pluginsdk.ExecutorProfileReader { return executorProfileReader{h} }

type workflowReader struct{ host *Host }

func (r workflowReader) List(_ context.Context, workspaceID string, _ pluginsdk.Page) ([]pluginsdk.Workflow, *pluginsdk.PageInfo, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	var result []pluginsdk.Workflow
	for _, workflow := range r.host.workflows {
		if workflow.WorkspaceID == workspaceID {
			result = append(result, workflow)
		}
	}
	return result, &pluginsdk.PageInfo{}, nil
}

func (r workflowReader) ListSteps(_ context.Context, workflowID string) ([]pluginsdk.WorkflowStep, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	return append([]pluginsdk.WorkflowStep(nil), r.host.steps[workflowID]...), nil
}

type agentProfileReader struct{ host *Host }

func (r agentProfileReader) List(context.Context, pluginsdk.Page) ([]pluginsdk.AgentProfile, *pluginsdk.PageInfo, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	return append([]pluginsdk.AgentProfile(nil), r.host.agentProfiles...), &pluginsdk.PageInfo{}, nil
}

type executorProfileReader struct{ host *Host }

func (r executorProfileReader) List(context.Context, pluginsdk.Page) ([]pluginsdk.ExecutorProfile, *pluginsdk.PageInfo, error) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	return append([]pluginsdk.ExecutorProfile(nil), r.host.executorProfiles...), &pluginsdk.PageInfo{}, nil
}
