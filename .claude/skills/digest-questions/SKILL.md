---
name: digest-questions
description: Extract what agent sessions were trying to establish and attach it to their facet records. Use when qm digest list --pending shows records without questions, or when asked to annotate the facet corpus so qm gaps has something to cluster on.
---

# Annotating facets with the questions a session was answering

`qm digest` derives everything a transcript shows structurally: how long the
agent searched before it landed anything, which knowledge documents it opened,
what the session produced. It cannot derive what the session was *trying to
establish*, because that is a judgment about meaning rather than a count.

That is this job. Read a transcript, name the questions, pipe them back.

## 1. Find the work

```
qm digest list --pending            # this repository
qm digest list --pending --all      # every repository on this machine
```

Each line is `session-id`, repository, transcript path. A record already
carrying questions is not listed, so this is a work queue that shrinks.

If a session is not listed at all it has not been digested yet. Run
`qm digest --backfill` first.

## 2. Read the transcript

The transcript is JSONL, one entry per line, and it is large. Do not read it
whole. What matters is the agent's own reasoning and the shape of the tool calls
that followed it:

```
jq -r 'select(.type=="assistant") | .message.content[]?
       | select(.type=="thinking") | .thinking' <transcript> | head -100
```

For the tool sequence:

```
jq -r 'select(.type=="assistant") | .message.content[]?
       | select(.type=="tool_use") | "\(.name)\t\(.input.file_path // .input.pattern // .input.command // "")"' \
  <transcript> | head -80
```

**Reasoning text is a candidate generator, not evidence.** It is the agent's
account of what it was doing, which is a rationalization rather than a cause.
Trust the tool sequence over the narration when they disagree, and treat a
reasoning span with no exploration behind it as a claim rather than a question.

Some sessions have no reasoning at all, depending on configuration. Then work
from the tool sequence alone: a spread of reads and greps converging on one file
is a question, and what the agent did next says whether it was answered.

## 3. Name the questions

A question is something the session had to establish before it could act. Aim
for one to five per session. A session that did one obvious thing has one
question or none, and none is a legitimate answer.

Write the question the way the agent would have asked it, not as a topic:

- Good: `why does the event envelope keep the raw payload verbatim`
- Bad: `event envelope design`

State what was being established, not what area it was in. The whole point is
that two sessions asking the same thing in different words end up in the same
cluster, and a topic label cannot do that.

`resolution` is how it actually got answered:

| Value | Means |
| --- | --- |
| `store_read` | A knowledge document under `.quartermaster/knowledge/` answered it |
| `source_read` | Reading the codebase answered it |
| `bash_exploration` | Running things answered it |
| `asked_human` | The agent gave up and asked |
| `unresolved` | It was never answered |

`tool_calls` is roughly how many calls went into that question. An estimate is
fine; it is used for ranking, not arithmetic.

`store_docs_read` lists document ids, only when a knowledge document was
actually opened for that question.

## 4. Attach it

```
echo '{"questions": [
  {"question": "why does the event envelope keep the raw payload verbatim",
   "resolution": "source_read", "resolved": true, "tool_calls": 14},
  {"question": "which package owns retry policy",
   "resolution": "unresolved", "resolved": false, "tool_calls": 6}
]}' | qm digest annotate <session-id>
```

A bare array works too. Re-annotating replaces the questions rather than adding
to them, so a correction is just another run.

## Do not

- Do not invent a question the transcript does not support. An empty list is a
  better record than a plausible fabrication, because the whole corpus exists to
  be evidence and one made-up cluster is worse than a missing one.
- Do not paste transcript content anywhere except into your own reading. The
  questions you write are the only thing that lands in a record, and transcripts
  hold credentials people pasted and things they said about colleagues.
- Do not use a resolution outside the table. `qm digest annotate` rejects it, on
  purpose: an open set cannot be clustered.
- Do not annotate a session you have not read the tool sequence for.
- Do not edit anything else in the record. The structural fields are what the
  transcript showed; they are not yours to revise.