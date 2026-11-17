---
name: company-research
description: >-
  Methodology for researching a company's online presence, structure,
  offerings, certifications, and financials. Produces a comprehensive
  French company profile following a structured template.
iconName: building-2
icon: "🏢"
hashtags: [research, company, french]
category: research
---

# Company Research Methodology

A structured approach to building a complete company profile from public web sources.

## Critical Rules

1. **Live data only** — You MUST use `web-search`, `web-fetch`, and `wikipedia` tools for EVERY piece of information. Do NOT use your training knowledge. Your training data is outdated and unreliable for company-specific facts.
2. **No placeholders** — NEVER write "[À remplir]" or "[À compléter]". If you cannot find the information after searching, write "[Information non trouvée — recherches effectuées : ...]" listing the queries you tried.
3. **Source everything** — Every fact in the profile MUST come from a tool call. If you didn't search for it, don't write it.
4. **Minimum tool usage** — You MUST call:
   - `web-search` at least **5 times** with different queries
   - `web-fetch` (action=scrape) at least **3 times** on the most important pages found
   - `wikipedia` at least **once** to check if an article exists
5. **Extract social media from homepage** — The company homepage footer almost always contains social media links (LinkedIn, Twitter/X, YouTube, GitHub, etc.). Scrape the homepage and extract them.
6. **Search in both languages** — Search in French AND English for maximum coverage.

## Research Phases

### Phase 1: Discover online presence

Use `web-search` to find all relevant company links. Execute ALL of these searches (adapt company name):

1. `"[company]" site officiel` — find main website
2. `"[company]" LinkedIn` — find LinkedIn page
3. `"[company]" chiffre d'affaires 2024` — find revenue/financial data
4. `"[company]" certification ISO 27001` — find security certifications
5. `"[company]" societe.com OR pappers.fr` — find legal registry entries

Additional searches if relevant:
- `"[company]" Wikipedia`
- `"[company]" Glassdoor avis`
- `"[company]" GitHub`
- `"[company]" certification HDS SOC2`

### Phase 2: Extract detailed information

For each discovered source, use `web-fetch` (action=scrape) to extract content. You MUST scrape at least these:

1. **Company homepage** — Extract: description, social media links (footer), navigation structure
2. **Financial/legal registry** (societe.com, pappers.fr) — Extract: SIREN, legal form, revenue, employee count
3. **Certification or compliance page** — Extract: certificate numbers, scope, expiry dates

Also use:
- `wikipedia` (action=summary) for the company name
- `wikipedia` (action=search) if summary returns nothing — try alternate names

**Key data points to extract**:
- Founding year, headquarters, legal form, SIREN/SIRET
- CEO/leadership team names and roles
- Number of employees
- Revenue (chiffre d'affaires) — most recent year available
- Products and services offered
- Key clients and partners
- Security certifications (number, scope, expiry date)
- ALL social media and web presence URLs
- CSR/sustainability initiatives

### Phase 3: Compile the profile

Organize all findings using the template below. Write in **French**. Cite source URLs for each section.

## Company Profile Template

```markdown
# Présentation de l'Entreprise : [Nom]

## 1. Fiche d'Identité
| Attribut | Valeur |
|----------|--------|
| Raison sociale | |
| Forme juridique | |
| SIREN/SIRET | |
| Date de création | |
| Siège social | |
| Dirigeant(e) | |
| Effectif | |
| Chiffre d'affaires | |
| Secteur d'activité | |
| Site web | |

## 2. Résumé Exécutif
- Description succincte
- Mission et vision
- Propositions de valeur (USPs)

## 3. Historique et Direction
- Étapes clés et réalisations
- Équipe de direction (noms, rôles)

## 4. Produits et Services
- Gammes principales
- Caractéristiques et avantages
- Innovation et R&D

## 5. Marché et Positionnement
- Secteur et tendances
- Clients cibles
- Principaux concurrents
- Stratégie de différenciation

## 6. Certifications et Sécurité
| Certification | Numéro | Périmètre | Date d'expiration |
|---------------|--------|-----------|-------------------|
| | | | |

## 7. Données Financières
- Chiffre d'affaires (dernières années disponibles)
- Indicateurs de croissance
- Investissements récents

## 8. Présence Digitale
| Canal | URL |
|-------|-----|
| Site web | |
| LinkedIn | |
| Twitter/X | |
| YouTube | |
| GitHub | |
| Wikipedia | |
| Glassdoor | |

## 9. RSE et Développement Durable
- Engagements environnementaux
- Initiatives sociales

## 10. Perspectives
- Objectifs stratégiques
- Plans d'expansion

---
Sources : [liste des URLs consultées avec la date de consultation]
Date de recherche : [date du jour]
```

## Quality Criteria

- Every section must have real data from web searches or "[Information non trouvée]"
- Financial data must include the year/period and source URL
- Certifications must include certificate number and expiry date when findable
- The "Présence Digitale" table must have actual URLs — scrape the homepage footer
- All source URLs must be listed in the Sources section
- The profile must be entirely in French
