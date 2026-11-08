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
| `/virements` | E-03 Virements | À venir |
| `/objectif` | E-04 Objectif | À venir |
| `/reglages` | E-05 Réglages | ✅ Palier, multiplicateur, plafonds |
| `/reglages/banque` | E-06 Banque (retour de la banque `?status=`) | Messages de retour en place |
| `/reglages/compte` | E-07 Compte | À venir |
| `/admin` | E-08 Administration | À venir |
