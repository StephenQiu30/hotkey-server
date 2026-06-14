package jobs

// This file previously contained adapters for the daily digest job.
// The new implementation uses the interfaces directly from the digest, llm,
// and obsidian packages, so adapters are no longer needed.
//
// The PublishDailyTopicsJob now uses:
// - *digest.Service for digest selection
// - llm.Client interface for LLM summarization
// - TopicExporter interface for export tracking
// - VaultWriter interface for Obsidian vault writes
