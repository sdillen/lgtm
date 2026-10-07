# Domänendokumentation

Wie die Engineering-Skills die Domänendokumentation dieses Repos konsumieren sollen, wenn sie die Codebasis erkunden.

## Vor dem Erkunden lesen

- **`CONTEXT.md`** im Repository-Stamm, oder
- **`CONTEXT-MAP.md`** im Repository-Stamm, falls vorhanden – sie verweist auf eine `CONTEXT.md` pro Kontext. Lese jede, die für das Thema relevant ist.
- **`docs/adr/`** – lies ADRs, die den Bereich betreffen, an dem du gerade arbeiten wirst. In Mehrfachkontext-Repos prüfe auch `src/<context>/docs/adr/` auf kontextspezifische Entscheidungen.

Wenn keine dieser Dateien existiert, **fahre still fort**. Weise nicht auf ihr Fehlen hin; schlage nicht vor, sie im Voraus zu erstellen. Der `/domain-modeling`-Skill (erreichbar über `/grill-with-docs` und `/improve-codebase-architecture`) erstellt sie bei Bedarf, wenn Begriffe oder Entscheidungen tatsächlich geklärt werden.

## Dateistruktur

Einzelkontext-Repo (die meisten Repos):

```
/
├── CONTEXT.md
├── docs/adr/
│   ├── 0001-event-sourced-orders.md
│   └── 0002-postgres-fuer-schreibmodell.md
└── src/
```

Mehrfachkontext-Repo (Vorhandensein von `CONTEXT-MAP.md` im Stamm):

```
/
├── CONTEXT-MAP.md
├── docs/adr/                          ← systemweite Entscheidungen
└── src/
    ├── bestellungen/
    │   ├── CONTEXT.md
    │   └── docs/adr/                  ← kontextspezifische Entscheidungen
    └── abrechnung/
        ├── CONTEXT.md
        └── docs/adr/
```

## Das Vokabular des Glossars verwenden

Wenn deine Ausgabe ein Domänenkonzept benennt (in einem Issue-Titel, einem Refaktor-Vorschlag, einer Hypothese, einem Testnamen), verwende den Begriff, wie er in `CONTEXT.md` definiert ist. Drifte nicht zu Synonymen ab, die das Glossar explizit vermeidet.

Wenn das benötigte Konzept noch nicht im Glossar ist, ist das ein Signal – entweder erfindest du Sprache, die das Projekt nicht verwendet (überdenken), oder es gibt eine echte Lücke (für `/domain-modeling` notieren).

## ADR-Konflikte kennzeichnen

Wenn deine Ausgabe einer bestehenden ADR widerspricht, bringe es explizit zur Sprache, anstatt stillschweigend zu überschreiben:

> _Widerspricht ADR-0007 (Event-Sourced Orders) – aber eine Wiedereröffnung wert, weil …_
