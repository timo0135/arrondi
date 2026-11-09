# Arrondi – PWA

Application web installable (PWA) d'Arrondi : Vue 3, Vite, TypeScript, Pinia, Vue Router, Tailwind CSS 4. Elle parle uniquement à l'API Go (`../api`) via `/api/v1`.

## Démarrage

Prérequis : Node 20+ et l'API qui tourne en local (`cd ../api && make run`).

```sh
cd web
npm install
npm run dev        # http://localhost:5173, /api relayé vers http://localhost:8080
```

En dev, l'API est en `APP_ENV=dev` : tu es connecté d'office, sans Cloudflare.

## Commandes

| Commande | Effet |
| --- | --- |
| `npm run dev` | Serveur de développement avec rechargement à chaud |
| `npm run build` | Vérification des types puis build de production dans `dist/` (avec service worker) |
| `npm run preview` | Sert le build de production en local |
| `npm run typecheck` | Vérification des types (`vue-tsc`) |
| `npm test` | Tests unitaires (Vitest) |
| `npm run gen:api` | Régénère `src/api/schema.d.ts` depuis l'API locale ; à lancer après chaque changement de route |

## Organisation

```
web/
├── public/              icônes et logos de la charte, manifest généré au build
├── src/
│   ├── api/
│   │   ├── schema.d.ts  types générés depuis l'OpenAPI de l'API (ne pas modifier)
│   │   └── client.ts    client openapi-fetch : X-Requested-With, erreurs, session expirée
│   ├── lib/
│   │   ├── format.ts    montants « 1 284,50 € », dates « 7 nov. 2026 », saisie en euros
│   │   ├── labels.ts    libellés bancaires lisibles, motifs d'exclusion en français
│   │   └── useAsync.ts  état de chargement d'un écran
│   ├── stores/          Pinia (utilisateur connecté)
│   ├── components/      icônes, barre d'onglets, en-tête, états
│   ├── views/           un fichier par écran (E-01 à E-08)
│   ├── router.ts
│   └── styles.css       couleurs et typographie de la charte (thèmes clair et sombre)
└── vite.config.ts       proxy /api, manifest PWA
```

## Règles

- **Montants en centimes** partout ; affichage par `formatCents`, saisie par `parseEuros`. Jamais de calcul en float.
- **Couleurs de la charte uniquement** (`bg-surface`, `text-ink`, `bg-primary`…), définies dans `styles.css` pour les deux thèmes ; le thème suit le réglage du téléphone.
- Un montant est toujours en `ink` avec la classe `amount` (chiffres tabulaires).
- Un seul bouton principal (`btn-primary`) par écran ; zones tactiles de 44 px minimum.
- Le service worker ne met jamais l'API en cache : la base serveur reste la source de vérité.
- `display: standalone` dans le manifest est obligatoire : sur iPhone, seule une PWA ajoutée à l'écran d'accueil garde ses données au-delà de 7 jours.

## Écrans

| Route | Écran | État |
| --- | --- | --- |
| `/` | E-01 Accueil | ✅ Bandeaux d'état, total, mois, « J'ai viré », objectif, dernière synchro |
| `/operations` | E-02 Opérations | ✅ Mois par mois, groupées par jour, arrondis et exclusions |
| `/virements` | E-03 Virements | ✅ Reste à virer, historique 12 mois, virements enregistrés |
| `/objectif` | E-04 Objectif | ✅ Progression, nom, montant, date facultative |
| `/reglages` | E-05 Réglages | ✅ Palier, multiplicateur, plafonds |
| `/reglages/banque` | E-06 Banque | ✅ Assistant en 4 étapes, état, compte suivi, synchro, déconnexion ; retour de la banque `?status=` |
| `/reglages/compte` | E-07 Compte | ✅ Export JSON, suppression avec confirmation |
| `/admin` | E-08 Administration | ✅ Membres, activation, invitation |

## Version ordinateur (D-20 à D-23)

Mêmes routes, autre mise en page, choisie en CSS et par `useDesktop()` / `useTablet()` / `useWide()` (`src/lib/useDesktop.ts`) :

| Largeur | Mise en page |
| --- | --- |
| < 768 px | Téléphone : une colonne, onglets en bas (écrans E-01 à E-08) |
| 768 à 1 023 px | Onglets en bas, contenu sur 720 px ; l'accueil devient le tableau de bord |
| ≥ 1 024 px | Barre latérale (`SideNav`), contenu en grille jusqu'à 1 240 px, sous-navigation des réglages (`SettingsNav`), feuilles du bas en boîtes de dialogue centrées |
| ≥ 1 280 px | Le détail d'une opération s'ouvre dans un panneau à droite au lieu d'une boîte de dialogue |

| Route | Écran | Sur ordinateur |
| --- | --- | --- |
| `/` | D-01 Tableau de bord | 4 indicateurs, épargne par mois (`BarChart`), le mois en chiffres, dernières opérations, où tu arrondis le plus |
| `/operations` | D-02 Opérations | Totaux du mois, filtre Toutes / Arrondies / Exclues, recherche (touche `/`), tableau, détail du calcul (`TxDetail`, Échap pour fermer), export CSV |
| `/virements` | D-03 Virements | Épargné et viré sur 6 mois, tableau par mois, formulaire prérempli affiché en place |
| `/objectif` | D-04 Objectif | Indicateurs, courbe cumulée avec projection (`GoalChart`), formulaire à droite |
| `/reglages` | D-05 Réglages | Brouillon et aperçu (`GET /settings/preview`), enregistrement au clic sur « Enregistrer » |
| `/reglages/banque` | D-06 Banque | Connexion, compte suivi, 10 dernières synchronisations |
| `/reglages/compte` | D-07 Mon compte | Contenu de E-07 avec la sous-navigation |
| `/admin` | D-08 Administration | Compteurs, tableau des membres, ajout en place, jamais de montant |

Graphiques : SVG faits maison, sans bibliothèque (D-22), avec texte alternatif et valeur exacte au survol ou au clavier. La version ordinateur ajoute environ 19 Ko gzip au total (budget : 25 Ko).
