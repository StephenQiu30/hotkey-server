# Knowledge Vault Fixtures

This directory contains fixture data for the Obsidian knowledge vault integration tests.

## Structure

```
knowledge_vault/
  README.md       # This file — fixture documentation
```

The actual vault structure is generated dynamically in `tests/integration/knowledge_vault_test.go`
using `BuildKnowledgePath`, `RenderEventNote`, `RenderDigestNote`, `RenderThemeNote`,
`RenderTopicNote`, and `WriteAtomic`.

## Generated Structure

When the integration test runs, it creates the following structure under a temp directory:

```
{TMP}/HotKey/
  events/{monitor-slug}/{date}-{id}-{title}.md
  topics/{monitor-slug}/{date}-topic-{id}-{slug}.md
  digests/daily/{monitor-slug}/{date}-{id}.md
  themes/{id}-{slug}.md
```

Each `.md` file follows the frontmatter contract used by the knowledge-vault exporter.
