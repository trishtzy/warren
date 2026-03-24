package middleware

// AgentKeyForTest returns the context key used for AgentInfo.
// This is exported only for use in handler tests.
func AgentKeyForTest() any {
	return agentKey
}
