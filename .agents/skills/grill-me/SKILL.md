---
name: grill-me
description: Explicit workflow for interrogating a proposed goal or plan one question at a time until shared understanding — the middle move of an open-spec session, before seal-spec writes the result. Use when the user explicitly says grill-me, invokes $grill-me, or asks to be grilled before writing a project/ spec change.
---

# Grill Me

Interview the user relentlessly about every aspect of this plan, until you reach
a shared understanding. Resolve each question one by one. With each
question provide your recommendation and reasoning. If the question can be
answered through exploration of the codebase or associated documents, do that
instead.

## Procedure

1. Surface one question at a time. Do not batch questions — resolve each fully
   before moving to the next.
2. Before asking, check whether the answer is discoverable. If exploring the
   codebase or associated documents can settle it, do that instead of asking,
   then report what you found.
3. **Before recommending any shape, check how this codebase already does it.**
   Whenever a question touches a surface the project has solved before — how a
   thing is constructed, named, configured, laid out on disk, how errors are
   shaped, how a seam is tested, how a fixture is built — read the neighbouring
   components that already solve it, and match them. Report what you found as
   part of the recommendation. If you are proposing a *departure* from the
   established pattern, say so explicitly and justify why this case earns the
   inconsistency; never present a departure as though it were the house style.
   Taste is not evidence here; reading the neighbours is.
4. When a question genuinely needs my input, state your recommendation and the
   reasoning behind it, then ask.
5. Keep going relentlessly until every aspect of the plan is resolved and we
   have a shared understanding. Do not stop early because the plan "seems good
   enough."
6. When there are no unresolved questions left, stop asking questions. Say
   plainly that the grilling is complete and ask how the operator wants to
   proceed. Do not frame the next workflow step as another grill question, do
   not recommend a next step unless asked, and do not offer direct
   implementation as an option during spec authoring.

## Why step 3 exists

A grilling can interrogate *what* to build exhaustively and still produce a
defective spec, because "is this how we already do it?" is a different question
from "is this a good idea?", and only the second comes up naturally in
conversation. When a spec invents a shape the codebase already has, the build
loop tends to follow the codebase — leaving the spec describing something that
was never built. That drift reads like an implementation error but originated
in authoring. Reading the neighbours is cheap and removes the whole class.
