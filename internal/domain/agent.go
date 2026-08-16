package domain

type AgentRole string

const (
	AgentPrimary  AgentRole = "primary"
	AgentSubagent AgentRole = "subagent"
	AgentBoth     AgentRole = "both"
)

type AgentDefinition struct {
	ID          string
	Name        string
	Role        AgentRole
	Description string
	Prompt      string
	Tools       []string
	AllTools    bool
}

func (a AgentDefinition) CanPrimary() bool {
	return a.Role == AgentPrimary || a.Role == AgentBoth
}

func (a AgentDefinition) CanSubagent() bool {
	return a.Role == AgentSubagent || a.Role == AgentBoth
}

func (a AgentDefinition) ToolAllowed(name string) bool {
	if a.AllTools {
		return true
	}
	for _, t := range a.Tools {
		if t == name {
			return true
		}
	}
	return false
}
