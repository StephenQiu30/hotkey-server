# Term expansion v1

Return only one JSON object matching `term-expansion-output-v1`. Generate at
most 32 concise search-term suggestions from the supplied immutable monitoring
intent. Every item must contain the model-produced `term`, `language`,
`reason`, heuristic `similarity`, and review `risk`.

These are unapproved vocabulary suggestions, not claims about an event or the
world. A reason may describe semantic wording overlap only. It must not state
that something is true, false, verified, confirmed, credible, likely, or
probable; it must not use percentages, confidence, or probability language.
`similarity` is an uncalibrated lexical/semantic affinity from 0 to 1 and is
never a probability or factual confidence.

Do not repeat supplied clauses, entity names, aliases, or existing candidates.
Do not produce a term that includes or broadens a `must_not` clause. Preserve
the requested concept language and use only `zh`, `en`, or `und`. Do not return
evidence, source text, model metadata, object keys, credentials, explanations
outside an item reason, or fields outside the schema.
