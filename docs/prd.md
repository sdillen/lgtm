# PRD: lgtm – Code-Review-TUI für agentische Workflows

## Problembeschreibung

Ich lasse einen Großteil meiner Code-Änderungen von Agenten (Claude Code, pi, …) erstellen. Meine Rolle verschiebt sich dadurch vom Schreiben zum **Verstehen und Bewerten** von Änderungen. Die vorhandenen Werkzeuge passen dazu schlecht:

- `git diff` und klassische Diff-Viewer zeigen nur Hunks. Um eine Änderung wirklich zu verstehen, brauche ich den umgebenden Code, Definitionen und Aufrufer – und muss dafür ständig in den Editor wechseln.
- Git-TUIs (lazygit, tig, …) sind auf das *Bedienen* von git ausgelegt (stagen, committen, rebasen), nicht auf das konzentrierte *Lesen* eines Reviews.
- Web-Reviews (Gitea/GitHub PRs) setzen einen Push und einen PR voraus und passen nicht zum schnellen lokalen Loop „Agent ändert → ich prüfe → Agent bessert nach“.
- Meine Anmerkungen muss ich mühsam von Hand formulieren und dem Agenten mit Datei- und Zeilenangaben zurückgeben. Nach der nächsten Runde muss ich wieder den kompletten Diff lesen, obwohl der Agent vielleicht nur zehn Zeilen geändert hat.
- Viele Tools sind visuell überladen; ich lese Code am liebsten ohne Ablenkung und bin vim/nvim-Bindings gewohnt.

## Lösung

`lgtm` („looks good to me“) ist eine schnelle, schöne Terminal-UI in Go zum **Lesen und Kommentieren** von Code-Änderungen – gezielt für agentische Workflows.

- Ohne Argumente zeigt `lgtm` alles, was seit dem Abzweig vom Default-Branch passiert ist – Commits **und** uncommittete Änderungen. Explizite Ranges oder einzelne Commits gehen als Argument.
- Dateien werden standardmäßig als **ganze Datei** mit hervorgehobenen Änderungen und eingeklappten unveränderten Bereichen gezeigt. Kontext ist damit nur einen Fold entfernt; ein Unified-Modus ist umschaltbar.
- Der Bildschirm gehört dem Code (**Zen-Mode**): eine dezente Statuszeile, Dateiliste und Picker nur als Overlays.
- Navigation fühlt sich an wie nvim: Hunks, Dateien und Kommentare per `]`/`[`, Folds per `z`, Wortsuche, projektweite Suche, heuristisches Go-to-Definition, Jumplist – auch in Dateien außerhalb des Diffs.
- Kommentare werden inline mit Kategorie (`must`/`should`/`nit`/`question`) auf Zeilen, Bereiche, Dateien oder das ganze Review gesetzt. Sie landen sofort in einer Markdown-Datei im Repo, die der Agent lesen kann, und lassen sich als fertiger, per Template anpassbarer Prompt in die Zwischenablage kopieren.
- Über mehrere **Runden** merkt sich `lgtm` den zuletzt reviewten Stand. In der nächsten Runde zeigt ein **Interdiff** nur das, was sich seitdem geändert hat; alte Kommentare sind sichtbar und abhakbar.
- Optik: Tokyo Night Day/Night, automatische Hell/Dunkel-Erkennung, Intra-Line-Highlighting, Smooth Scrolling, Minimap-Scrollbar.
- Der Exit-Code meldet, ob das Review approved wurde – `lgtm` kann damit später auch als blockierender Schritt in Agenten-Workflows dienen.

## User Stories

### Review-Gegenstand

1. Als Reviewer möchte ich `lgtm` ohne Argumente starten und alle Änderungen seit dem Abzweig vom Default-Branch sehen, inklusive uncommitteter Änderungen, damit ich ohne Nachdenken die komplette Arbeit des Agenten prüfe.
2. Als Reviewer möchte ich, dass der Default-Branch automatisch erkannt wird (z. B. `main`/`master` bzw. der HEAD des Remotes), damit ich ihn nicht angeben muss.
3. Als Reviewer möchte ich eine explizite Range (`a..b`) angeben, damit ich gezielt eine Commit-Spanne prüfen kann.
4. Als Reviewer möchte ich einen einzelnen Commit angeben, damit ich genau dessen Änderungen sehe.
5. Als Reviewer möchte ich `lgtm` in einem Linked Worktree starten, damit ich die Arbeit von Agenten prüfen kann, die in eigenen Worktrees arbeiten.
6. Als Reviewer möchte ich eine verständliche Fehlermeldung bekommen, wenn ich `lgtm` außerhalb eines Git-Repos starte oder eine Range nicht auflösbar ist, damit ich weiß, was falsch ist.
7. Als Reviewer möchte ich einen freundlichen Hinweis statt einer leeren Ansicht sehen, wenn es keine Änderungen gibt, damit ich sofort weiß, dass nichts zu tun ist.

### Startscreen und Überblick

8. Als Reviewer möchte ich beim Start einen kurzen Überblick sehen (Branch, Base, Runde, Anzahl Dateien, `+X −Y`, Verteilung pro Verzeichnis), damit ich den Umfang der Änderung sofort einschätzen kann.
9. Als Reviewer möchte ich, dass der Startscreen beim ersten Tastendruck verschwindet, damit er mich nicht aufhält.

### Diff-Darstellung

10. Als Reviewer möchte ich standardmäßig die ganze neue Datei mit hervorgehobenen Änderungen sehen, damit ich Änderungen in ihrem Kontext lese.
11. Als Reviewer möchte ich, dass unveränderte Bereiche eingeklappt sind und als „⋯ N unveränderte Zeilen ⋯“ erscheinen, damit ich mich auf Änderungen konzentriere.
12. Als Reviewer möchte ich Folds mit `zo`/`zc`/`zR`/`zM` öffnen und schließen, damit ich Kontext nach Bedarf erweitere.
13. Als Reviewer möchte ich in eine Unified-Ansicht umschalten, damit ich bei Bedarf die klassische Diff-Sicht habe.
14. Als Reviewer möchte ich gelöschte Zeilen inline an ihrer ursprünglichen Stelle sehen, damit ich verstehe, was ersetzt wurde.
15. Als Reviewer möchte ich Syntax-Highlighting für alle gängigen Sprachen, damit Code gut lesbar ist.
16. Als Reviewer möchte ich auch gelöschte Zeilen gedämpft syntax-gehighlightet sehen, damit sie lesbar bleiben.
17. Als Reviewer möchte ich innerhalb geänderter Zeilen die tatsächlich geänderten Wörter/Tokens kräftiger hervorgehoben sehen, damit ich kleine Änderungen sofort erkenne.
18. Als Reviewer möchte ich neue Dateien ohne flächig eingefärbten Hintergrund, nur mit Gutter-Markierung, sehen, damit sie lesbar bleiben.
19. Als Reviewer möchte ich umbenannte Dateien als `alt → neu` mit nur dem inhaltlichen Diff sehen, damit Refactorings nicht als riesige Lösch-/Hinzufüge-Blöcke erscheinen.
20. Als Reviewer möchte ich gelöschte Dateien standardmäßig eingeklappt sehen und bei Bedarf den alten Inhalt aufklappen, damit sie mich nicht ablenken.
21. Als Reviewer möchte ich bei Binärdateien nur Metadaten (Größe alt/neu) sehen, damit die Ansicht nicht kaputtgeht.
22. Als Reviewer möchte ich Mode-Änderungen und Submodul-Änderungen als kompakte Info-Einträge sehen, damit ich sie nicht übersehe.
23. Als Reviewer möchte ich, dass generierte Dateien, Lockfiles und Vendor-Verzeichnisse (inkl. `linguist-generated` in `.gitattributes`) eingeklappt ans Ende sortiert werden, damit Agenten-Rauschen mich nicht aufhält.
24. Als Reviewer möchte ich lange Zeilen standardmäßig nicht umbrechen und horizontal mit `zl`/`zh` scrollen, damit Einrückung lesbar bleibt.
25. Als Reviewer möchte ich Softwrap per `:set wrap` einschalten können, damit ich bei Prosa/Markdown alles sehe.
26. Als Reviewer möchte ich Whitespace-only-Änderungen per `:set ignorews` ausblenden können, damit Formatierungsrauschen verschwindet.
27. Als Reviewer möchte ich auch sehr große Dateien flüssig scrollen können, damit das Tool bei großen Agenten-Diffs nicht träge wird.

### Layout und Zen-Mode

28. Als Reviewer möchte ich standardmäßig nur Code und eine dezente Statuszeile sehen, damit ich ohne Ablenkung lese.
29. Als Reviewer möchte ich in der Statuszeile Dateipfad, Position in der Dateiliste, `+/−`-Statistik, Kommentaranzahl und Runde sehen, damit ich orientiert bleibe.
30. Als Reviewer möchte ich eine Dateiliste als Drawer ein- und ausblenden, damit ich bei Bedarf den Überblick bekomme.
31. Als Reviewer möchte ich die Dateiliste als Baum mit Status (ungelesen / gelesen / kommentiert / geändert seit letzter Runde) sehen, damit ich weiß, was noch offen ist.
32. Als Reviewer möchte ich optional eine permanente Sidebar einschalten können, damit ich auf großen Bildschirmen beides sehe.
33. Als Reviewer möchte ich eine Minimap-Scrollbar sehen, die anzeigt, wo in der Datei Änderungen und Kommentare liegen, damit ich mich in langen Dateien orientiere.
34. Als Reviewer möchte ich optional Nerd-Font-Icons in der Dateiliste aktivieren, damit es bei passender Schrift schöner aussieht, ohne dass es ohne sie kaputt aussieht.

### Navigation

35. Als Reviewer möchte ich mit `j`/`k`, `<C-d>`/`<C-u>`, `gg`/`G`, `zz`/`zt` und Counts navigieren, damit sich das Tool wie nvim anfühlt.
36. Als Reviewer möchte ich mit `]c`/`[c` zum nächsten/vorherigen Hunk springen, damit ich Änderungen zügig durchgehe.
37. Als Reviewer möchte ich mit `]f`/`[f` zur nächsten/vorherigen Datei springen, damit ich das Review linear abarbeite.
38. Als Reviewer möchte ich mit `]k`/`[k` zum nächsten/vorherigen Kommentar springen, damit ich meine Anmerkungen wiederfinde.
39. Als Reviewer möchte ich, dass Sprünge weich animiert scrollen (abschaltbar), damit ich die Orientierung behalte.
40. Als Reviewer möchte ich einen Fuzzy-Picker über die geänderten Dateien, damit ich direkt zu einer Datei springe.
41. Als Reviewer möchte ich einen Fuzzy-Picker über **alle** Dateien des Repos, damit ich beliebigen Kontext read-only öffnen kann.
42. Als Reviewer möchte ich mit `/`, `n`, `N` in der aktuellen Datei suchen, damit ich Stellen schnell finde.
43. Als Reviewer möchte ich mit `*`/`#` nach dem Wort unter dem Cursor suchen, damit ich Verwendungen in der Datei finde.
44. Als Reviewer möchte ich mit `gr` projektweit nach dem Wort unter dem Cursor suchen und eine Ergebnisliste sehen, damit ich Aufrufer und Verwendungen finde.
45. Als Reviewer möchte ich mit `gd` zur (heuristisch ermittelten) Definition des Symbols unter dem Cursor springen, damit ich verstehe, was eine Funktion oder ein Typ tut.
46. Als Reviewer möchte ich mit `<C-o>`/`<C-i>` durch meine Sprunghistorie zurück und vor gehen, damit ich nach einem Abstecher wieder dort lande, wo ich war.
47. Als Reviewer möchte ich in Dateien außerhalb des Diffs immer den neuen Stand sehen, damit der Kontext zur Änderung passt.
48. Als Reviewer möchte ich nach `<space>`, `]`, `[`, `g`, `z` nach kurzer Wartezeit ein which-key-Popup mit den möglichen Folgetasten sehen, damit ich Bindings nicht auswendig wissen muss.
49. Als Reviewer möchte ich mit `?` eine vollständige Hilfe sehen, damit ich alle Bindings nachschlagen kann.
50. Als Reviewer möchte ich eine `:`-Kommandozeile für seltene Aktionen (`:theme`, `:set`, `:lgtm`, `:q`), damit nicht alles eine Taste braucht.

### Gelesen-Status

51. Als Reviewer möchte ich, dass eine Datei als gelesen markiert wird, sobald ich bis zum Ende gescrollt habe, damit ich meinen Fortschritt sehe.
52. Als Reviewer möchte ich den Gelesen-Status manuell umschalten können, damit ich ihn korrigieren kann.
53. Als Reviewer möchte ich, dass der Gelesen-Status zurückgesetzt wird, wenn sich die Datei seit der letzten Runde geändert hat, damit ich nichts Neues übersehe.

### Kommentare

54. Als Reviewer möchte ich mit `c` einen Kommentar zur aktuellen Zeile schreiben, damit ich Anmerkungen schnell festhalte.
55. Als Reviewer möchte ich mit `V` einen Zeilenbereich markieren und kommentieren, damit sich der Kommentar auf einen Block bezieht.
56. Als Reviewer möchte ich eine ganze Datei kommentieren, damit ich strukturelle Anmerkungen machen kann.
57. Als Reviewer möchte ich einen globalen Kommentar zum gesamten Review schreiben, damit ich übergreifende Hinweise geben kann.
58. Als Reviewer möchte ich auch unveränderten Kontext-Code kommentieren, damit ich auf vorhandenen Code verweisen kann (z. B. „nutze diese bestehende Funktion“).
59. Als Reviewer möchte ich gelöschte Zeilen kommentieren, damit ich sagen kann „das hätte nicht entfernt werden dürfen“.
60. Als Reviewer möchte ich jedem Kommentar eine Kategorie (`must`/`should`/`nit`/`question`) geben und sie mit `<Tab>` wechseln, damit der Agent priorisieren kann.
61. Als Reviewer möchte ich Kommentare in einem Inline-Float mit minimalen vim-Bindings schreiben, damit ich im Lesefluss bleibe.
62. Als Reviewer möchte ich mit `<C-s>` speichern und mit `<Esc><Esc>` verwerfen, damit das Schreiben schnell geht.
63. Als Reviewer möchte ich Kommentare inline als virtuelle Zeilen unter dem Code sehen, in der Farbe ihrer Kategorie, damit ich sie im Kontext lese.
64. Als Reviewer möchte ich einen Kommentar unter dem Cursor mit `c` bearbeiten, mit `dd` (nach Bestätigung) löschen und mit `x` als erledigt markieren, damit ich ihn pflegen kann. Erledigte Kommentare sind gedimmt, durchgestrichen und mit `✓` markiert, damit ich sie auf einen Blick von offenen unterscheide; die Statuszeile zählt sie (`x/y erledigt`).

### Export zum Agenten

65. Als Reviewer möchte ich, dass jeder Kommentar sofort in `.lgtm/review.md` geschrieben wird, damit nichts verloren geht und der Agent jederzeit lesen kann.
66. Als Reviewer möchte ich, dass jeder Kommentar dort mit `datei:zeilenbereich`, zitiertem Code und Kategorie erscheint, damit der Agent ihn eindeutig verorten kann.
67. Als Reviewer möchte ich, dass `.lgtm/` automatisch in `.git/info/exclude` eingetragen wird, damit es nie versehentlich committet wird.
68. Als Reviewer möchte ich mit `Y` alle offenen Kommentare als fertigen Prompt in die Zwischenablage kopieren, damit ich ihn direkt in meine Agenten-Session einfüge.
69. Als Reviewer möchte ich mit `yc` nur den Kommentar unter dem Cursor kopieren, damit ich einzelne Punkte gezielt weitergeben kann.
70. Als Reviewer möchte ich, dass die Zwischenablage auch über SSH und in Multiplexern funktioniert (OSC 52 mit Fallback auf lokale Tools), damit es in jeder Umgebung klappt.
71. Als Reviewer möchte ich den Prompt per Template anpassen (global und projektlokal, lokal hat Vorrang), damit er zu meinen Agenten und Konventionen passt.
72. Als Reviewer möchte ich das eingebaute Default-Template ausgeben lassen, damit ich einen Startpunkt für eigene Templates habe.
73. Als Reviewer möchte ich, dass `must`-Kommentare im Prompt zuerst kommen und `question` als „nur beantworten, nichts ändern“ markiert ist, damit der Agent richtig priorisiert.
74. Als Reviewer möchte ich beim Beenden mit offenen Kommentaren einen Hinweis bekommen („3 offene Kommentare – `Y` kopieren?“), damit ich das Kopieren nicht vergesse.
75. Als Reviewer möchte ich mit `:lgtm` approven, einen kurzen Bestätigungsmoment sehen und `lgtm` mit Exit-Code 0 beenden, damit Skripte und Agenten erkennen, dass das Review bestanden ist. Nach dem Bestätigungsmoment wird der Review-Zustand wie bei `lgtm clear` gelöscht (`.lgtm/`, `refs/lgtm/`), damit das nächste Review frisch beginnt. Bei offenen Kommentaren fragt `:lgtm` zuerst nach („N offene Kommentare werden verworfen“); erst das zweite `:lgtm` approved.
76. Als Agenten-Workflow möchte ich einen Exit-Code ungleich 0 bekommen, wenn beim Beenden offene Kommentare existieren, damit ich weiß, dass nachgebessert werden muss.

### Review-Runden und Interdiff

77. Als Reviewer möchte ich, dass `lgtm` beim Review den reviewten Stand als Snapshot festhält, damit spätere Runden darauf aufbauen können.
78. Als Reviewer möchte ich in einer neuen Runde auf eine Interdiff-Ansicht umschalten, die nur Änderungen seit meinem letzten Review zeigt, damit ich nicht alles erneut lesen muss.
79. Als Reviewer möchte ich zwischen Gesamt-Diff und Interdiff hin- und herschalten, damit ich bei Bedarf den Gesamtkontext sehe.
80. Als Reviewer möchte ich Kommentare der vorherigen Runde mit `Runde N ·` gekennzeichnet in ihrer Kategoriefarbe sehen und als erledigt markieren können, damit ich prüfen kann, ob der Agent sie umgesetzt hat. Erst erledigte Kommentare werden gedimmt, durchgestrichen und mit `✓` dargestellt.
81. Als Reviewer möchte ich, dass nur offene Kommentare exportiert werden, damit der Agent keine erledigten Punkte erneut bearbeitet.
82. Als Reviewer möchte ich `lgtm` jederzeit schließen und später an derselben Stelle weiterreviewen, damit ich Reviews unterbrechen kann.

### Live-Änderungen

83. Als Reviewer möchte ich in der Statuszeile einen Hinweis bekommen, wenn sich Dateien während des Reviews ändern, damit ich weiß, dass der Agent weiterarbeitet.
84. Als Reviewer möchte ich mit `R` bewusst neu laden, damit mir der Code nicht unter dem Cursor wegspringt.
85. Als Reviewer möchte ich, dass beim Neuladen Cursor und Kommentar-Anker über den zitierten Code wiedergefunden werden, damit meine Kommentare an der richtigen Stelle bleiben.
86. Als Reviewer möchte ich Kommentare, deren Code nicht mehr gefunden wird, als „verwaist“ markiert sehen statt gelöscht, damit nichts stillschweigend verschwindet.

### Theming und Konfiguration

87. Als Nutzer möchte ich, dass `lgtm` automatisch erkennt, ob mein Terminal einen hellen oder dunklen Hintergrund hat, damit es sich ohne Konfiguration einfügt.
88. Als Nutzer möchte ich das Theme per Config, `--theme` oder `:theme` überschreiben, damit ich die Kontrolle behalte.
89. Als Nutzer möchte ich Tokyo Night Day und Tokyo Night Night als eingebaute Themes, damit es von Anfang an schön aussieht.
90. Als Nutzer möchte ich eigene Themes als TOML mit semantischen Tokens (Diff, UI, Kommentar-Kategorien, Syntax-Basis plus Overrides) anlegen, damit ich das Aussehen anpassen kann.
91. Als Nutzer möchte ich, dass `lgtm` auf Terminals ohne Truecolor automatisch auf 256 Farben zurückfällt, damit es überall funktioniert.
92. Als Nutzer möchte ich meine Konfiguration unter `$XDG_CONFIG_HOME/lgtm/` ablegen, damit sie zu meinen Dotfiles passt.
93. Als Nutzer möchte ich Keybindings inklusive Leader-Taste per Config umbelegen, damit ich sie an meine nvim-Gewohnheiten anpassen kann.
94. Als Nutzer möchte ich, dass which-key und Hilfe meine umbelegten Keys zeigen, damit die Doku stimmt.
95. Als Nutzer möchte ich Smooth Scrolling und Icons per Config abschalten können, damit ich die Optik an mein Terminal anpasse.

### Installation

96. Als Nutzer möchte ich `lgtm` per `go install` installieren, damit es ohne weitere Abhängigkeiten (kein cgo, kein git-Binary) läuft.
97. Als Nix-Nutzer möchte ich `lgtm` als Nix-Paket bauen, damit es in meine Umgebung passt.
98. Als Nutzer möchte ich `lgtm --version` aufrufen, damit ich weiß, welche Version ich nutze.
99. Als Nutzer möchte ich `lgtm --help` aufrufen, damit ich Aufruf, Flags und Exit-Codes nachschlagen kann.
100. Als Nutzer oder Agenten-Workflow möchte ich mit `lgtm clear` den Zustand des aktuellen Reviews (`.lgtm/` und die Snapshot-Refs unter `refs/lgtm/`) löschen, damit ich neu beginnen kann; `clear` fragt nach, ohne Terminal ist `--yes` nötig.

## Implementierungsentscheidungen

### Technologie

- Go, **kein cgo**, keine Laufzeitabhängigkeit auf das `git`-Binary.
- **go-git v6** (gepinnt, mindestens alpha.5 wegen der behobenen Symlink-CVE im Worktree) für Objektzugriff, Refs, Merge-Base, Snapshots und Working-Tree-Status. Linked Worktrees werden mit gemeinsamem `.git`-Verzeichnis geöffnet.
- **znkr.io/diff** für Zeilen- und Token-Diffs (Intra-Line). Die Diff-Berechnung erfolgt selbst, nicht über go-git.
- **Bubble Tea v2, Lip Gloss v2, Bubbles v2** (`charm.land/...`), **harmonica** für Smooth Scrolling.
- **Chroma** für Syntax-Highlighting.
- **fsnotify** für den Watcher.
- TOML für Konfiguration und Themes, `text/template` für den Prompt.
- Modulpfad `gitea.fit.fraunhofer.de/ste05096/lgtm`.

### Module (tiefe Module, schmale Interfaces)

- **source** – kapselt jeglichen Git-Zugriff. Löst den Review-Gegenstand auf (Default: Merge-Base mit dem Default-Branch bis Working Tree; alternativ Range oder einzelner Commit) und liefert pro geänderter Datei alte und neue Version samt Metadaten (Status, Mode, binär, Submodul). Liefert beliebige Repo-Dateien im neuen Stand, legt Snapshots als Refs unter `refs/lgtm/…` an und löst sie wieder auf, und meldet Änderungen über den Watcher. Schnittstelle so schmal, dass später ein git-CLI-Backend daneben passt.
- **diff** – nimmt alte und neue Version und erzeugt ein `ReviewFile`-Modell: die Zeilenfolge der Full-File-Ansicht (Kontext, hinzugefügt, gelöscht) mit Intra-Line-Bereichen, Hunks, Fold-Regionen und Zeilennummern alt/neu. Enthält Rename-Erkennung (Ähnlichkeit ≥ 50 %), Whitespace-Ignorieren und die Klassifizierung als generiert/Lockfile/Vendor (inkl. minimaler `.gitattributes`-Auswertung). Rein funktional, keine I/O.
- **review** – Domänenmodell eines Reviews: Kommentare mit Anker (Datei + Zeilenbereich im neuen Stand, oder gelöschte Zeilen im alten Stand, oder Datei, oder global), Code-Zitat, Kategorie und Status (offen/erledigt/verwaist), Runden, Snapshots und Gelesen-Status. Kernfähigkeit: **Anker-Neuverortung** gegen eine neue Dateiversion anhand des Zitats. Persistiert `.lgtm/state.json` (Maschinenzustand) und `.lgtm/review.md` (für den Agenten) bei jeder Änderung und pflegt den Eintrag in `.git/info/exclude`.
- **export** – rendert offene Kommentare über das Prompt-Template (eingebaut, global, projektlokal) und schreibt in die Zwischenablage (OSC 52, Fallback `wl-copy`/`xclip`/`pbcopy`).
- **search** – Wortsuche in Datei und Projekt (respektiert `.gitignore`) sowie heuristisches Go-to-Definition über sprachspezifische Definitionsmuster.
- **theme** – lädt eingebaute und eigene Themes, erkennt Hell/Dunkel, bildet semantische Tokens und Chroma-Stile auf Lip-Gloss-Styles ab.
- **keymap** – Präfix-Trie der Bindings inklusive Leader, Counts und Config-Overrides. Aus derselben Struktur entstehen Dispatch, which-key-Popup und `?`-Hilfe.
- **ui** – Bubble-Tea-Modelle für Startscreen, Datei-Ansicht, Drawer, Picker, Kommentar-Float, which-key, Kommandozeile und Statuszeile. Die UI bekommt fertige `ReviewFile`-Modelle und ruft nur Domänen-Methoden auf; sie enthält keine Git- oder Diff-Logik. Highlighting erfolgt lazy für den sichtbaren Bereich plus Puffer.
- **cmd** – CLI-Einstieg, Flags, Argument-Parsing, `--help`, Subcommand `clear`, Exit-Codes, Versionsinfo.

### Verträge und Formate

- `.lgtm/review.md`: nach Datei gruppiert; jeder Kommentar mit Kopf `datei:start-ende` (bzw. Vermerk „alt, entfernt“), Kategorie, zitiertem Code-Block mit Sprach-Tag und Kommentartext. Globale Kommentare zuerst. Nur offene Kommentare.
- `.lgtm/state.json`: versioniertes Schema mit Runden (Snapshot-Ref, Zeitpunkt), Kommentaren (stabile ID, Anker, Zitat, Kategorie, Status, Runde) und Gelesen-Status pro Datei (inkl. Hash des gelesenen Inhalts).
- Snapshots: der reviewte Stand inklusive Working-Tree-Änderungen wird als Tree/Commit-Objekt unter `refs/lgtm/…` abgelegt, damit er vor Garbage Collection geschützt ist.
- Prompt-Template-Daten: Branch, Base, Runde, Kommentare (sortiert nach Kategorie und gruppiert nach Datei) plus Hilfsfunktionen zum Gruppieren und Sortieren.
- Exit-Codes: 0 nach `:lgtm` bzw. ohne offene Kommentare, 1 bei offenen Kommentaren, >1 bei Fehlern.

### Interaktion

- Zen-Mode ist Default. Drawer, Picker, Kommentar-Float und which-key sind Overlays mit abgerundeten Rahmen; der Hintergrund wird dabei abgedunkelt.
- Watcher ändert die Ansicht nie automatisch; nur Hinweis plus manuelles Neuladen.
- Gelesen-Markierung wird automatisch gesetzt, wenn das Dateiende sichtbar war.

## Testentscheidungen

- Gute Tests prüfen **nur externes Verhalten** über die öffentlichen Schnittstellen der Module (z. B. „gegeben alte und neue Datei, liefert `ReviewFile` diese Zeilen/Folds/Intra-Line-Bereiche“), nicht interne Datenstrukturen oder private Funktionen. Refactorings dürfen Tests nicht brechen.
- **TDD** für die Kernmodule `source`, `diff`, `review`, `export`, `search`, `keymap` und `theme`.
- **Fixture-Repos** werden im Test per go-git **in-memory** aufgebaut (Commits, Branches, Worktree-Änderungen) – kein Shell-git, keine Binär-Fixtures im Repo.
- Die **Anker-Neuverortung** bekommt die gründlichste Suite: verschobene Zeilen, Zeilen davor eingefügt/gelöscht, geänderter Inhalt, doppelt vorkommende Zitate, gelöschte Dateien, Renames, verwaiste Anker.
- `review.md`- und Prompt-Ausgaben werden mit **Golden Files** geprüft.
- Die **UI** wird mit `teatest` und Golden Files gerenderter Frames bei fester Terminalgröße und festem Theme für die Kernansichten getestet; keine Abdeckung aller Zustände.
- Die Scroll-Performance in großen Dateien wird mit Benchmarks früh gemessen.
- Es gibt noch keinen Code; die Tests begründen die Konventionen. Das Repo nutzt devenv mit `gofmt`- und `go test`-Hooks und `golangci-lint`.

## Außerhalb des Umfangs

- Jegliches Git-Bedienen: stagen, committen, checkout, rebase, push.
- Code bearbeiten in `lgtm`.
- Side-by-side-Ansicht.
- LSP-Integration (exaktes Go-to-Definition, Referenzen, Hover).
- Tree-sitter-basiertes Highlighting und strukturelle Folds.
- Erkennung verschobenen Codes (moved code).
- Bildvorschau für Binärdateien (Kitty/Sixel).
- Direktes Senden an laufende Agenten-Sessions und stdout-Export; nur Datei und Zwischenablage.
- Kommentare über `$EDITOR`.
- Remote-PR-Integration (Gitea/GitHub-Reviews lesen oder schreiben).
- Marks, Makros und weitere fortgeschrittene vim-Features.
- Ein git-CLI-Backend (nur als spätere Option vorbereitet).
- Distribution über goreleaser, Homebrew o. ä.

## Weitere Hinweise

- **Hauptrisiken**: die Anker-Neuverortung (Interdiff und Reload), der Alpha-Status von go-git v6 (abgefedert durch das schmale `source`-Interface) und die Scroll-Performance mit Highlighting und Animation.
- Linked Worktrees sind ein Kern-Szenario, weil Agenten häufig in eigenen Worktrees arbeiten.
- Das Kommentarformat ist bewusst agentenneutral (Markdown mit `datei:zeilen` und Zitat), damit es mit Claude Code, pi und anderen Agenten funktioniert.
- Mögliche Erweiterungen danach: LSP, Tree-sitter, Moved-Code-Erkennung, Senden an Agenten-Sessions, git-CLI-Backend für sehr große Repos, Veröffentlichung auf GitHub.
