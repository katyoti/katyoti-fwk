# Audit de Code : ktw-code

> Sidecar d'analyse de code — scan de dossiers, extraction de symboles, cartographie des dépendances.

---

## 1. Vue d'Ensemble

| Métrique | Valeur |
|----------|--------|
| Langages | Go (source), Markdown (documentation embarquée), YAML (configuration/CWM) |
| Fichiers source Go | 4 fichiers |
| Lignes de code Go | 97 lignes au total |
| Fichiers Markdown embarqués | 50 fichiers |
| Fichiers YAML embarqués | 8 fichiers |
| Fichiers de test | 0 (aucun fichier `*_test.go`) |
| Taille du dépôt (sous-dossier) | ~472 Ko |
| Dernier commit | `4edb5723 updates` |
| Module Go (racine du monorepo) | `github.com/katyoti/katyoti-fwk` — Go 1.26 |

---

## 2. Structure du Projet

### Arborescence principale

```
app/ktw-code/
├── main.go              ← Point d'entrée — délègue à cmd.Execute(embeddedFS)
├── embed.go             ← Embarque les dossiers config/, skills/, references/, cwm/ via go:embed
├── README.md            ← Documentation courte (modules composés, CLI surface)
└── cmd/
│   ├── root.go          ← Execute() → appcore.BootstrapCLI("sidecar", …)
│   └── bootstrap.go     ← Déclaration de toutes les extensions (sidecarkit builder)
└── embed/
    ├── config/
    │   └── CodeService.yaml   ← Configuration du service (extensions, taille max, répertoires exclus)
    ├── skills/
    │   ├── code-review.md     ← Skill : orchestration d'une revue de code complète (multi-agents)
    │   ├── code-module.md     ← Skill : méthode module-rock-solid (Bootstrap → Promote)
    │   ├── code-explain.md    ← Skill : explication de code
    │   ├── code-fast-doc.md   ← Skill : documentation rapide
    │   └── usage.md           ← Guide d'utilisation des skills
    ├── tasks/
    │   ├── code-fast-doc.md   ← Tâche : documentation automatique
    │   └── spec-extract.md    ← Tâche : extraction de spécifications
    ├── references/
    │   ├── project-overview.md, tech-stack.md, configuration.md, tooling.md
    │   ├── modules/            ← Références de la méthode module-rock-solid (workflow, test-layers, …)
    │   │   ├── README.md
    │   │   ├── workflow.md, charter-template.md, test-layers.md, e2e-test-authoring.md, …
    │   │   └── verbs/          ← Références par verbe (charter, status, lint, verify, audit, promote, …)
    │   └── review/             ← Guides de revue (architecture, security, quality, complexity, …)
    └── cwm/
        ├── node_types/         ← module.yaml, audit-bundle.yaml
        ├── item_types/         ← test-run.yaml, charter.yaml, audit-finding.yaml
        └── edge-types/         ← realized-by.yaml, closes.yaml
```

### Nature du projet

`ktw-code` est un **sidecar binaire Go** appartenant au monorepo `ktw-agents`. Il ne contient pas de logique métier en propre : c'est une coquille d'assemblage qui :
1. **Embarque** la base documentaire (skills, tâches, références, types CWM) via `go:embed`
2. **Enregistre** les extensions via le builder `sidecarkit`
3. **Expose** une CLI structurée sous la commande `code` (`ktw-code code analyze-folder / get-symbol / dependency-graph`)

La logique métier réelle (analyse de code) vit dans `internal/extensions/code/` et `internal/extensions/code/local/` au niveau du monorepo.

---

## 3. Dépendances

### Dépendances directes (depuis `go.mod` racine)

Le `go.mod` est partagé par l'ensemble du monorepo. Les dépendances clés pour `ktw-code` spécifiquement :

| Dépendance | Rôle dans ktw-code |
|---|---|
| `github.com/spf13/cobra v1.10.2` | Framework CLI (via `appcore.BootstrapCLI`) |
| `github.com/nats-io/nats.go v1.52.0` | Transport NATS pour la communication sidecar (KTSP bus) |
| `github.com/nats-io/nats-server/v2 v2.14.0` | Serveur NATS embarqué |
| `gopkg.in/yaml.v3 v3.0.1` | Lecture des fichiers YAML de configuration et CWM |
| `github.com/modelcontextprotocol/go-sdk v0.8.0` | MCP bridge |
| `github.com/google/uuid v1.6.0` | Identifiants uniques |

### Dépendances directes du monorepo (toutes) — synthèse

- **47 dépendances directes** déclarées dans `go.mod`
- Catégories représentées : CLI (Cobra), IA/LLM (Anthropic, OpenAI, Gemini/Vertex, Cohere), transport (NATS, WebSocket, gRPC), UI/TUI (Bubbletea, Charm), navigateur (ChromeDP), fichiers (rclone, excelize), observabilité (OpenTelemetry, Prometheus)
- Aucun fichier `go.sum` ou `vendor/` visible dans le sous-dossier (gérés au niveau monorepo racine)

---

## 4. Qualité du Code

### Couverture de tests

| Indicateur | Valeur |
|---|---|
| Fichiers source Go | 4 |
| Fichiers de test `*_test.go` | **0** |
| Couverture estimée | **0%** |

> ⚠️ **Aucun test unitaire dans ce package.** Cela est cohérent avec la nature du binaire : `ktw-code` est une coquille d'assemblage (bootstrap + embed), et les tests métier vivent probablement dans `internal/extensions/code/` et les suites L2/L3 du monorepo. Ce n'est pas forcément problématique, mais mérite d'être noté.

### TODOs / FIXMEs

| Pattern | Occurrences dans `*.go` |
|---|---|
| `TODO` | 0 |
| `FIXME` | 0 |
| `HACK` | 0 |
| `XXX` | 0 |

Le code Go est propre — aucune dette technique annotée dans les 97 lignes source.

### Qualité du code Go

- **main.go** (15 lignes) : minimaliste et correct. Gestion d'erreur standard (`fmt.Fprintln(os.Stderr, err)` + `os.Exit(1)`).
- **embed.go** (6 lignes) : directive `go:embed` bien formée avec `all:` pour le dossier `references` (préserve les fichiers `.` cachés si présents).
- **cmd/root.go** (14 lignes) : délégation propre à `appcore.BootstrapCLI`.
- **cmd/bootstrap.go** (62 lignes) : correctement commenté, architecture claire. La décision de NE PAS appeler `WithFlatCLI` est documentée inline — bonne pratique.

### Documentation embarquée

La richesse documentaire embarquée est remarquable :
- **50 fichiers Markdown** couvrant skills, tâches, références méthodologiques, guides de revue
- **8 fichiers YAML** définissant les types CWM (ontologie du modèle de travail)
- Références organisées en sous-arbres découvrables progressivement (`verbs/`, `review/`, `templates/`)

---

## 5. Points d'Attention Sécurité

### Analyse des patterns à risque

| Pattern | Résultat |
|---|---|
| Secrets hardcodés (`password`, `api_key`, `secret`, `token`) dans les `.go` | **Aucun** |
| Patterns dangereux (`eval(`, `exec(`, `dangerouslySetInnerHTML`, `sql.Query`) | **Aucun** |
| Fichiers `.env` committés | **Aucun** |
| Variables de version | `var version = "dev"` (valeur de dev, normale pour un build sans ldflags) |

### Fichiers sensibles et .gitignore

Le `.gitignore` racine est bien configuré :
- `.env`, `.env.local`, `*.env` sont exclus ✅
- `vendor/`, `node_modules/`, `/bin/`, `/dist/` sont exclus ✅
- `.secrets/`, `.config/sessions/`, `.config/logs/` sont exclus ✅
- Les binaires compilés (`*.exe`, `*.dll`, `/ktw-*.exe`) sont exclus ✅

### Observations spécifiques à ktw-code

1. **Bus NATS** : la communication inter-sidecars passe par NATS. La sécurité de ce canal (auth, TLS) dépend de la configuration du déploiement — non visible ici.
2. **`languagePackResolverExtension`** : enregistre un proxy `BusLanguagePack` vers `ktw-gocode`. Le sens de confiance de ce canal devrait être documenté dans la charter de sécurité.
3. **`go:embed all:embed/references`** : le flag `all:` embarque également les fichiers préfixés par `.` — à surveiller si des fichiers sensibles venaient à être ajoutés dans ce dossier.
4. **`version = "dev"`** : en production, ce champ devrait être injecté via `-ldflags "-X main.version=..."` au build. Sinon tous les binaires s'identifient comme `dev`.

---

## 6. Recommandations

### Points forts ✅

1. **Architecture ultra-minimaliste et claire** : 97 lignes Go suffisent pour bootstrapper un sidecar complet. Le patron `sidecarkit.NewSidecar(…)` est élégant.
2. **Documentation de très haute qualité** : 50 fichiers Markdown bien structurés, progressifs, avec des références croisées explicites. La méthode `module-rock-solid` est documentée de façon exhaustive.
3. **Zéro dette technique annotée** dans le code Go.
4. **Séparation claire** entre la coquille d'assemblage (ce package) et la logique métier (`internal/extensions/code/`).
5. **`.gitignore` complet** et bien géré au niveau monorepo.
6. **Commentaires inline utiles** dans `bootstrap.go` (pourquoi `WithFlatCLI` est absent, la topologie sidecar B7, la migration SN-12).

### Améliorations suggérées 🔧

1. **Tests d'intégration pour la coquille** : même si la logique est dans `internal/`, un test vérifiant que `buildExtensions()` ne panique pas et que les commandes attendues (`code analyze-folder`, etc.) sont bien enregistrées apporterait une filet de sécurité.
2. **Injection de version au build** : ajouter une target `Makefile` / script CI qui injecte la version via `-ldflags "-X github.com/katyoti/katyoti-fwk/app/ktw-code/cmd.version=$(git describe --tags)"`.
3. **Documenter les prérequis de déploiement NATS** : un `docs/deploy.md` minimal listant les variables d'environnement attendues (adresse du bus, credentials NATS) serait utile.
4. **Lint de la directive `go:embed`** : ajouter un test ou une vérification CI qui s'assure que tous les chemins embarqués existent bien (évite des paniques silencieuses au démarrage si un fichier est renommé).
5. **CHANGELOG** : les commits sont tous libellés `updates` — un CHANGELOG.md ou des tags sémantiques faciliteraient le suivi des évolutions.

### Risques identifiés ⚠️

| Risque | Sévérité | Détail |
|---|---|---|
| Absence de tests dans ce package | Faible | Cohérent avec la nature de coquille, mais un smoke-test de bootstrap serait prudent |
| Version `dev` en production potentielle | Faible | Si les binaires ne sont pas buildés avec `-ldflags`, l'introspection de version sera toujours `dev` |
| Canal NATS non audité ici | Moyen | La sécurité du bus inter-sidecars dépend d'une configuration extérieure — à couvrir dans la charter sécurité |
| `go:embed all:` sur `references/` | Faible | Le flag `all:` embarque les fichiers `.`-préfixés ; à surveiller si ce dossier reçoit des fichiers sensibles |

---

*Date d'audit : 2026-06-26*  
*Périmètre : `app/ktw-code/` — coquille sidecar uniquement (hors `internal/extensions/code/` qui contient la logique métier réelle)*
