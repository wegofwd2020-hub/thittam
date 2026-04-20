# Consuming the Progress Chart Data

`docs/PROGRESS.md` and its sibling CSVs (`PROGRESS_scopes.csv`,
`PROGRESS_issues.csv`) are regenerated nightly by
[`.github/workflows/progress.yml`](../.github/workflows/progress.yml).

This page enumerates the options for getting that data in front of a
human. Each option has its own tradeoffs — pick based on audience and how
live the view needs to be.

---

## Currently active: Option C — workflow step summary

The nightly workflow writes a top-of-highlights summary to its own run
page via `$GITHUB_STEP_SUMMARY`. To see it:

1. Open [Actions → Progress Chart](../../actions/workflows/progress.yml)
2. Click the latest run
3. The summary block lives above the job logs

This gives "at-a-glance numbers" when you're in the repo checking on CI,
with zero extra hosting. It's per-run — there is no persistent URL for
"the latest summary." For a persistent dashboard, see Option A below.

---

## Option catalogue

### Option A — GitHub Pages dashboard

Build a small static site (HTML + JS + Chart.js / Plotly) that fetches
the CSVs from `raw.githubusercontent.com` and renders live charts. Host
free at `https://wegofwd2020-hub.github.io/thittam`.

**Pros.** Real dashboard; auto-updates as CSVs change; shareable URL;
supports hover/filter/drilldown.

**Cons.** You're maintaining a small frontend. 2-3 hours initial build.

**Setup sketch.**
1. Enable Pages on the repo (Settings → Pages → Source: `docs/` on `main`).
2. Add `docs/dashboard/index.html` that `fetch()`es the CSVs.
3. Parse with PapaParse, render with Chart.js.
4. No workflow changes — Pages rebuilds on every push to `main`.

### Option B — commit PNG charts nightly

Extend `generate_progress.py` to call `matplotlib` and write chart PNGs
alongside the markdown. The nightly workflow commits them. Embed in
`PROGRESS.md` via standard image tags.

**Pros.** No hosting. Renders directly in the GitHub UI. Still
auto-updated nightly.

**Cons.** Static images — no interactivity. Adds `matplotlib` as a
workflow dep. Commits .png files bloats the repo (~tens of KB per night).

**Setup sketch.**
1. `pip install matplotlib` in the workflow.
2. Extend `scripts/generate_progress.py` with a `render_charts()` step.
3. Commit `docs/PROGRESS_*.png` alongside the markdown.

### Option C — workflow step summary ✅ *active*

Append markdown tables to `$GITHUB_STEP_SUMMARY` in the `progress.yml`
workflow. Shows on the workflow run page.

**Pros.** Zero infra. ~30 min to set up. No new files committed.

**Cons.** Per-run only — no persistent landing page. Only visible to
people with repo access who know to navigate to Actions.

**Setup.** Already done — see [`progress.yml`](../.github/workflows/progress.yml).

### Option D — GitHub Projects v2

Create a Project board with a table view over issues. Add custom fields
for commit count, age, etc. Populate from the existing GitHub issues.

**Pros.** Built-in, free, good for work tracking. **Fits Thittam well**
because features already live as GitHub issues (unlike StudyBuddy where
tickets live in epic markdown files).

**Cons.** Still doesn't read the CSVs directly — it's issue-based, not
metric-based. Commit-count-per-issue would need to be pushed into a
custom Project field via a separate workflow.

**Verdict:** worth considering if you already use Projects for Thittam
planning; overkill otherwise.

### Option E — third-party BI (Metabase / Grafana / Datadog)

Connect a BI tool to the CSVs or the GitHub API. Build drilldown
dashboards with alerting.

**Pros.** Real analytics features, alerts, scheduled digests.

**Cons.** Outside GitHub; new credentials surface; hosting concerns
(self-host Metabase or pay for hosted); overkill for a single-person
project.

**Verdict:** revisit only if the team grows past ~5 people with
conflicting views on "what the data means."

---

## How to pick

| If your need is… | Use |
|---|---|
| Quick glance when I'm in the repo anyway | **C** (active) |
| Shareable dashboard URL for a stakeholder | A |
| Visual at the top of the README / PROGRESS.md | B |
| "Which issues have 2+ commits this quarter?" with filters | Google Sheets via `=IMPORTDATA(...)` (already set up) |
| Cross-project metrics + alerts | E |
| Work-tracking board tied to existing issues | D |

The CSV siblings support A, B, D, and the Sheets use case simultaneously
— adding one of those later doesn't break the others.
