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
whole.

**Do not go looking for the agent's reasoning. It is not there.** Thinking blocks
are present in the transcript but their text is stripped, so every one is an
empty string and only the signature survives. Anything built on reading them
returns nothing and looks like an empty session.

Four things do survive, in the order they are worth reading.

**What the person asked.** The clearest statement of what was in question.

```
jq -r 'select(.type=="user") | .message.content | select(type=="string")' <transcript> \
  | grep -v '^\s*$' | awk 'length<200' | head -40
```

Long prompts are usually pasted documents rather than requests, which is why
this filters them out. Read a few whole if the short ones do not make sense.

**Where the agent stopped and asked.** Read these, and expect most of them to be
decisions somebody had to make rather than things nobody could find. The record
counts them; promoting one to a question is a judgment you make here.

```
jq -r 'select(.type=="assistant") | .message.content[]?
       | select(.name=="AskUserQuestion") | .input.questions[]?.question' <transcript>
```

**What it delegated or looked up outside the codebase.** A subagent task, a
documentation lookup, or a web search is a question this repository could not
answer from itself. These are the gap candidates.

```
jq -r 'select(.type=="assistant") | .message.content[]?
       | select(.name=="Agent" or .name=="WebFetch" or .name=="WebSearch" or (.name|startswith("mcp__")))
       | "\(.name)\t\(.input.description // .input.url // .input.query // (.input|tostring))"' <transcript> | head -30
```

**The tool sequence.** What was actually done, and the only thing that says
whether a question was answered.

```
jq -r 'select(.type=="assistant") | .message.content[]?
       | select(.type=="tool_use") | "\(.name)\t\(.input.file_path // .input.pattern // .input.command // "")"' \
  <transcript> | head -80
```

A spread of reads and greps converging on one file is a question. What the agent
did next says whether it was answered. When the prompts and the tool sequence
disagree about what was going on, the tool sequence is what happened.

## 3. Name the questions

A question is something the session had to establish before it could act. Aim
for one to five per session. A session that did one obvious thing has one
question or none, and none is a legitimate answer.

**Only write down questions that could recur.** This is the filter that decides
whether the corpus is worth clustering, and it is the easiest one to skip. Ask
whether a different session, in a different repository, could plausibly have to
establish the same thing. "Does Claude Code support path-scoped rules" could;
"what Go module path should this project use" could not, and a one-time decision
recorded as a question is noise that never joins a cluster. Naming a decision is
not the job either: "should this field be renamed" was settled by taste, not by
looking anything up.

The record carries `human_asks`, a count of the times the agent stopped and put
something to a person. It is a count rather than a list on purpose: those are
usually decisions to be made rather than facts to be recovered, so most of them
fail the recurrence filter. Read them in the transcript, and promote one to a
question only when the agent asked because it genuinely could not find out.

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
| `external_docs` | A dependency's documentation, a web page, or an MCP lookup answered it |
| `delegated` | A subagent was sent to find out |
| `asked_human` | The agent gave up and asked |
| `unresolved` | It was never answered |

The last four are the interesting ones. `store_read` says the store worked.
`source_read` and `bash_exploration` say the answer was in the repository, so a
document about it would mostly be a stale copy. The other four say the answer
was somewhere this repository could not reach, which is what a content gap looks
like from the inside.

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
- Do not annotate the session you are in. Its transcript is still being written,
  so its span, counts, and outcome are all still moving, and the act of
  annotating adds to the file being read. Any other session can be annotated
  from here; transcripts and records are both user-scoped and neither belongs to
  the conversation doing the reading.
- Do not resume a past session to annotate it. Resuming appends to the very
  transcript that is the evidence, and loads a whole conversation's history to
  do work that only needs the file.
- Do not edit anything else in the record. The structural fields are what the
  transcript showed; they are not yours to revise.