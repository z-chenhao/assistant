export interface AgentRunRequest { task: string; artifactRefs?: readonly string[] }
export interface AgentResult { readonly text: string; readonly artifactRefs: readonly string[] }
