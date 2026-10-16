# lgtm

Code-Review-TUI für agentische Workflows. Anforderungen, Module und Testentscheidungen stehen im PRD: `docs/prd.md` – vor jeder Implementierungsarbeit lesen.

## Agent skills

### Issue-Tracker

Issues und PRDs leben als GitHub Issues in `sdillen/lgtm`, Zugriff über die GitHub-MCP-Tools; externe PRs sind keine Triage-Oberfläche. Siehe `docs/agents/issue-tracker.md`.

### Triage-Labels

Standard-Vokabular: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. Siehe `docs/agents/triage-labels.md`.

### Domänendokumentation

Einzelkontext: eine `CONTEXT.md` und `docs/adr/` im Repository-Stamm. Siehe `docs/agents/domain.md`.
