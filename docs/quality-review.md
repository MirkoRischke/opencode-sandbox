# Local readability review

Use this rubric when asking an agent to review a refactor. Supply the baseline
commit and the current revision or working tree. Metrics live in `.quality/`.

Read the complete changed functions and tests, including helpers they use, in
both versions. Review these criteria:

- Purpose: each test or subtest describes one coherent behavior.
- Visibility: relevant inputs, action, and expected outcome are clear locally.
- Helpers: mechanics are reusable; behavior-specific conditions remain visible.
- Structure: tables and subtests group scenarios with common setup and assertions.
- Failure clarity: failures identify the broken behavior and relevant values.
- Implementation: responsibilities and resource ownership are clear; extracted
  helpers have a meaningful purpose beyond lowering a complexity score.

For each concrete problem, record its category, function or test, evidence,
and status: introduced, resolved, or remaining. Match problems by behavior,
including across renamed functions. Inspect moved code and helpers before
declaring a problem resolved. Return an empty findings list when appropriate.

Save the review with the refactor's verification notes. Keep model and rubric
fixed for comparisons. Agent findings are judgments, not a numeric readability
score; numeric improvement alone does not prove readability improved.
