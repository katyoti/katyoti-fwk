# Audit de Code : ktw-demo

## 1. Vue d'Ensemble

| Métrique | Valeur |
|---|---|
| Langage principal | Go 1.26 (mono-module racine) |
| Fichiers Go | 4 (main.go, embed.go, cmd/root.go, cmd/bootstrap.go) |
| Lignes de code Go | **80 lignes** |
| Fichiers de tâches embarquées | 19 fichiers Markdown (embed/tasks/) |
| Fichier de type de données DSL | 1 YAML (book.dsl.yaml — 104 lignes) |
| Lignes Markdown totales | ~3 026 (tâches + README) |
| Fichiers de test | **0** (aucun test unitaire) |
| Taille du sous-répertoire | ~200 Ko |
| Dernier commit | 2026-06-23 — « updates » |
| Activité récente | 10 commits consécutifs nommés « updates » |

---

## 2. Structure du Projet

```
app/ktw-demo/
├── main.go                      # Point d'entrée — délègue à cmd.Execute(embeddedFS)
├── embed.go                     # //go:embed embed/tasks embed/data-types
├── cmd/
│   ├── root.go                  # Execute() → appcore.BootstrapCLI("sidecar", ...)
│   └── bootstrap.go             # buildExtensions() — déclare le sidecar via sidecarkit
├── embed/
│   ├── tasks/                   # 19 fichiers Markdown de définition de tâches
│   │   ├── hello-world.md       # Tâche simple (greeting + file-write)
│   │   ├── document-demo.md     # Démo create-document
│   │   ├── display-demo.md                   (495 lignes — le plus volumineux)
│   │   ├── display-drawio-demo.md            (406 lignes)
│   │   ├── display-data-binding-demo.md      (169 lignes)
│   │   ├── compose-ui-live-demo.md           (130 lignes)
│   │   ├── live-task-list-demo.md            (165 lignes)
│   │   ├── demo-ask-data-wizard.md           (130 lignes)
│   │   ├── demo-delegate-magentic.md         (156 lignes)
│   │   ├── demo-delegate-pipeline.md          (92 lignes)
│   │   ├── demo-delegate-parallel-fallback.md (76 lignes)
│   │   ├── demo-delegate-tree.md             (137 lignes)
│   │   ├── demo-delegate-team-tree.md        (269 lignes)
│   │   ├── demo-delegate-team-coordination.md (45 lignes)
│   │   ├── demo-delegate-team-messaging.md    (62 lignes)
│   │   ├── demo-crud-book.md                 (188 lignes)
│   │   ├── demo-crud-book-declarative.md     (113 lignes)
│   │   ├── demo-crud-cwm.md                  (137 lignes)
│   │   ├── via-agent-child.md                 (26 lignes — interne)
│   │   └── via-agent-test.md                  (25 lignes)
│   └── data-types/
│       └── book.dsl.yaml        # Schéma DSL du type Book (CRUD demos)
└── README.md                    # Documentation utilisateur (66 lignes)
```

### Architecture

`ktw-demo` est un **sidecar statique** du framework `ktw-agents`. Sa seule
responsabilité : livrer un catalogue de tâches et de workflows de démonstration
via `embed.FS` Go. Il ne démarre aucun serveur HTTP (`WithoutHTTPServer`),
n'expose aucune compétence (`WithoutSkills`) et s'appuie intégralement sur le
scaffolding standard `BootstrapCLI` / `sidecarkit`. Les 80 lignes de Go sont
du pur câblage de déclaration — toute la valeur est dans les fichiers embarqués.

### Catégories de tâches

| Catégorie | Tâches |
|---|---|
| Démos UI | display-demo, display-drawio-demo, display-data-binding-demo, compose-ui-live-demo, live-task-list-demo |
| Délégation / orchestration | demo-delegate-magentic, demo-delegate-pipeline, demo-delegate-parallel-fallback, demo-delegate-tree |
| Teams & messaging | demo-delegate-team-tree, demo-delegate-team-coordination, demo-delegate-team-messaging |
| CRUD schema-driven | demo-crud-book, demo-crud-book-declarative, demo-crud-cwm |
| Basique | hello-world, document-demo, demo-ask-data-wizard |
| Interne | via-agent-child, via-agent-test |

---

## 3. Dépendances

### Imports Go directs de ktw-demo

`ktw-demo` fait partie du **mono-module** `github.com/katyoti/katyoti-fwk` (go.mod racine, Go 1.26) — il n'a pas de `go.mod` propre. Il n'importe que trois packages internes :

| Package | Rôle |
|---|---|
| `github.com/katyoti/katyoti-fwk/internal/appcore` | Bootstrap du sidecar (BootstrapCLI) |
| `github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit` | Builder déclaratif (WithTasks, WithDataTypes…) |
| `github.com/katyoti/katyoti-fwk/internal/core/application` | Type `Extension` |

> **Zéro dépendance externe directe** — le module est du pur contenu statique.

### Dépendances notables du mono-module (héritées)

| Bibliothèque | Usage |
|---|---|
| `github.com/spf13/cobra` v1.10.2 | CLI |
| `github.com/nats-io/nats.go` v1.52.0 | Messagerie NATS |
| `github.com/anthropics/anthropic-sdk-go` v1.34.0 | LLM Anthropic |
| `github.com/openai/openai-go/v2` v2.7.1 | LLM OpenAI |
| `cloud.google.com/go/vertexai` v0.12.0 | LLM Google Vertex AI |
| `github.com/gorilla/websocket` v1.5.3 | WebSocket |
| `github.com/google/uuid` v1.6.0 | Identifiants UUID |
| `github.com/wailsapp/wails/v2` v2.11.0 | Frontend desktop |
| `gopkg.in/yaml.v3` v3.0.1 | Parsing YAML (schémas DSL) |
| `github.com/modelcontextprotocol/go-sdk` v0.8.0 | Protocole MCP |

---

## 4. Qualité du Code

### Couverture de tests

| Indicateur | Valeur |
|---|---|
| Fichiers `*_test.go` | **0** |
| Ratio test/source Go | **0 %** |
| Tests d'intégration Markdown | **0** |

L'absence de tests est acceptable pour ce module : les 4 fichiers Go (80 lignes)
ne font que câbler des déclarations de framework. La validation fonctionnelle
des tâches relève des tests bout-en-bout du mono-repo. En revanche, **aucun test
de cohérence structurelle** ne vérifie le frontmatter YAML des fichiers `.md`
embarqués, ce qui constitue un risque de régression silencieuse (cf. Recommandations).

### TODOs / FIXMEs

**Aucun** `TODO`, `FIXME`, `HACK` ou `XXX` trouvé dans l'ensemble du projet.

### Résultat des tests

Non exécutables (aucun fichier `*_test.go`). Le build est présumé valide puisque
le binaire est livré par le mono-repo.

### Qualité du code Go

- **Gestion d'erreurs correcte** : `main.go` utilise `fmt.Fprintln(os.Stderr, err)` + `os.Exit(1)`.
- **`embed.FS` bien utilisé** : la directive `//go:embed embed/tasks embed/data-types` est propre et ciblée.
- **Documentation interne** : le bloc de commentaire de `bootstrap.go` explique le rôle du sidecar, les choix architecturaux et les instructions pour activer les skills futurs. La date de migration (2026-05-23, référence ticket SN-12) est consignée.
- **Variable `version = "dev"`** : placeholder standard — doit être injectée via `-ldflags` au build (documenté dans README mais non vérifié automatiquement).

### Qualité du contenu embarqué

- Frontmatter YAML cohérent sur les 19 tâches (`name`, `type`, `description`, `icon`, `iconName`, `hashtags`, `category`, `config`).
- Schémas DSL `input-schema` / `output-schema` présents sur toutes les tâches applicables.
- Les tâches complexes documenter leur topologie, leur coût LLM estimé et les anti-patterns à éviter (ex. : `ledger-snapshot` vs `ledger-wait-update` dans `demo-delegate-magentic.md`).
- **Duplication consentie** : le schéma `Book` est défini à la fois dans `book.dsl.yaml` (registre) et en JSON inline dans `demo-crud-book.md`. C'est intentionnel (l'agent a besoin du JSON inline) mais crée un risque de désynchronisation.

---

## 5. Points d'Attention Sécurité

| Vérification | Résultat |
|---|---|
| Credentials en dur | ✅ Aucun trouvé |
| Clés API / secrets | ✅ Aucun trouvé |
| `eval()` / `exec()` / SQL non paramétré | ✅ Aucun trouvé |
| `dangerouslySetInnerHTML` | ✅ Aucun trouvé |
| Fichiers `.env` commités | ✅ Aucun trouvé |
| `.gitignore` local | ✅ Absent (couvert par le `.gitignore` racine du mono-repo) |
| Serveur HTTP exposé | ✅ Désactivé (`WithoutHTTPServer`) |

### Observations spécifiques

1. **`sessionToken` dans un diagramme DrawIO** (`display-drawio-demo.md`, ligne 160) : la chaîne `"- sessionToken: string"` apparaît dans un exemple XML de diagramme UML. Il s'agit d'un **attribut d'une classe `GuestSession`** dans un template de démo — ce n'est pas un secret réel.

2. **Données de seed en dur** : `demo-crud-book.md` contient des adresses email de démonstration (`thomas.hunt@example.com`, `robert.martin@example.com`, `martin.kleppmann@example.com`). Ce sont des données inventées pour la démo — aucun risque.

3. **`.gitignore` racine** : couvre correctement `.secrets/`, `.worktrees/`, `.config/sessions/`, `.config/logs/`, `bin/` et les répertoires IDE. Le sous-répertoire `ktw-demo` ne génère aucun artefact sensible.

4. **Surface d'attaque nulle** : le sidecar ne démarre aucun serveur, n'ouvre aucun port, ne traite aucune entrée réseau directe.

---

## 6. Recommandations

### ✅ Points forts

- **Architecture exemplaire** : séparation totale entre contenu (Markdown/YAML) et infrastructure (Go). Ajouter une tâche ne nécessite aucune modification de code — uniquement déposer un fichier `.md`.
- **Zéro dette technique** : aucun TODO, aucun FIXME, aucun code mort.
- **Documentation comme code** : chaque tâche est une spécification complète incluant pattern, topologie, coût LLM et anti-patterns. La documentation *est* le produit.
- **Couverture des patterns d'orchestration** : le catalogue couvre pipeline, parallèle, fallback, tree, magentic, team messaging, CRUD, UI DSL, DrawIO, data binding, wizard — référence exhaustive pour l'écosystème.
- **Opt-in propre** : les univers de production peuvent omettre ce sidecar sans impact.
- **Sécurité par conception** : pas de serveur, pas de secrets, pas de code dynamique.

### 🔧 Améliorations suggérées

1. **Tests de cohérence du contenu embarqué** : ajouter un fichier `cmd/embed_integrity_test.go` qui itère sur tous les fichiers `embed/tasks/*.md`, parse le frontmatter YAML et vérifie la présence des champs obligatoires (`name`, `type`, `description`). Coût : ~50 lignes de Go. Bénéfice : détection immédiate d'une régression de contenu à la CI, avant même l'exécution.

2. **Injection de version au build** : automatiser `-ldflags "-X github.com/katyoti/katyoti-fwk/app/ktw-demo/cmd.version=$(VERSION)"` dans le `Makefile` cible `ktw-demo`. Actuellement la version affichée vaut toujours `"dev"`, ce qui rend le diagnostic de version impossible en production.

3. **Messages de commit conventionnels** : les 10 derniers commits portent le message générique `"updates"`. Adopter les messages conventionnels (`feat:`, `fix:`, `docs:`, `chore:`) faciliterait le suivi des changements et la génération automatique de changelog.

4. **Réduire la duplication du schéma Book** : le type `Book` est défini deux fois — dans `book.dsl.yaml` (registre canonique) et en JSON inline dans `demo-crud-book.md` / `demo-crud-book-declarative.md`. Envisager de pointer vers `schemaName: "book"` (URI `ktw-demo:/data-type/book`) depuis les tâches, une fois que la résolution par nom sera supportée dans le contexte page-iframe (blocage technique noté dans `display-demo.md`).

5. **Nettoyer les hashtags de `hello-world`** : la tâche `hello-world` porte le tag `schema-driven-ui` qui n'est pas représentatif de son contenu (simple greeting + file write). Correction cosmétique mais améliore la précision du catalogue.

6. **Clarifier le statut de `via-agent-test.md`** : ce fichier (25 lignes) est très court et ne porte pas de `advertised: false`. Vérifier s'il est prêt pour le catalogue ou s'il doit être marqué comme tâche interne, comme l'est `via-agent-child.md`.

### ⚠️ Risques identifiés

| Risque | Sévérité | Probabilité | Mitigation |
|---|---|---|---|
| Frontmatter Markdown invalide non détecté | Faible | Moyenne | Ajouter `embed_integrity_test.go` |
| Schéma Book désynchronisé (YAML vs inline) | Faible | Faible | Refactorer vers `schemaName` quand supporté |
| Version `"dev"` en production | Faible | Haute | Automatiser l'injection via `ldflags` |
| Coût LLM non plafonné sur certaines démos (~$0.03) | Info | — | Déjà documenté dans les fichiers de tâches |

---

*Date d'audit : 2026-06-23*
*Auditeur : Analyse automatisée — Senior Software Engineer Agent*
*Périmètre : `app/ktw-demo/` (mono-repo `github.com/katyoti/katyoti-fwk`)*
