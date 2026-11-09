<script setup lang="ts">
// E-06 Banque. Sans connexion : assistant en 4 étapes (consentement, application Enable Banking,
// choix de la banque, autorisation). Avec connexion : état, compte suivi, consentement, synchro.
// Cette page est aussi l'adresse de retour après l'autorisation (?status=ok|refused|error).
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { bankName, banks, bankStatus } from '@/lib/bankStatus'
import { formatDate, formatSyncTime } from '@/lib/format'
import { useReconnect } from '@/lib/useReconnect'
import { useDesktop } from '@/lib/useDesktop'
import SettingsNav from '@/components/SettingsNav.vue'
import { useToast } from '@/stores/toast'
import AppIcon from '@/components/AppIcon.vue'
import BottomSheet from '@/components/BottomSheet.vue'
import PageTitle from '@/components/PageTitle.vue'
import StatusBanner from '@/components/StatusBanner.vue'
import TextField from '@/components/TextField.vue'

type Conn = Schemas['ConnectionOut']

const route = useRoute()
const router = useRouter()
const toast = useToast()
const { busy: reconnecting, reconnect } = useReconnect()

const conn = ref<Conn | null>(null)
const isDesktop = useDesktop()
// D-06 (ordinateur) : les 10 dernières synchronisations (EF-43).
const runs = ref<Schemas['SyncRunOut'][]>([])
async function loadRuns() {
  if (!isDesktop.value || !conn.value) return
  try {
    runs.value = (await call(api.GET('/bank/sync-runs', { params: { query: { limit: 10 } } }))) ?? []
  } catch {
    runs.value = []
  }
}
const triggerLabel = (t: string) => (t === 'manual' ? 'Manuelle' : 'Automatique')
function runResult(r: Schemas['SyncRunOut']) {
  if (r.status === 'running') return 'En cours'
  if (r.status === 'error') return r.error_code === 'rate_limited' ? 'Limite de la banque atteinte' : 'Banque ou Enable Banking indisponible'
  return `${r.fetched} lue${r.fetched > 1 ? 's' : ''} · ${r.inserted} nouvelle${r.inserted > 1 ? 's' : ''}`
}
const loaded = ref(false)
const loadError = ref<string | null>(null)

async function load() {
  loadError.value = null
  try {
    conn.value = await call(api.GET('/bank/connection'))
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) conn.value = null
    else loadError.value = e instanceof ApiError ? e.detail : 'Impossible de joindre le serveur.'
  } finally {
    loaded.value = true
  }
  await loadRuns()
}
onMounted(load)

// --- retour de la banque ---
const returnMessages: Record<string, { text: string; cls: string }> = {
  ok: { text: 'Ta banque est connectée. La première synchronisation aura lieu demain à 06:30, ou tout de suite si tu la lances.', cls: 'bg-primary-soft text-ink' },
  refused: { text: 'L’autorisation a été annulée chez ta banque. Tu peux recommencer quand tu veux.', cls: 'bg-warning-soft text-ink' },
  error: { text: 'La connexion n’a pas abouti. Réessaie dans un instant.', cls: 'bg-danger-soft text-danger' },
}
const returned = computed(() => returnMessages[String(route.query.status ?? '')])

// --- connexion existante ---
const tracked = computed(() => (conn.value?.accounts ?? []).find((a) => a.is_tracked) ?? null)
const status = computed(() => bankStatus[conn.value?.status ?? 'none']!)
const banner = computed(() => {
  switch (conn.value?.status) {
    case 'expired':
      return { tone: 'danger' as const, text: 'Ton consentement a expiré : les synchronisations sont arrêtées. Tes données restent consultables.', action: 'Reconnecter' }
    case 'error':
      return { tone: 'danger' as const, text: conn.value.last_error ?? 'La dernière synchronisation a échoué.', action: 'Reconnecter' }
    case 'expiring_soon':
      return { tone: 'warning' as const, text: `Ton consentement expire dans ${conn.value.expires_in_days ?? 0} jours.`, action: 'Renouveler' }
    case 'pending':
      return { tone: 'warning' as const, text: 'Tes identifiants sont enregistrés. Il reste à autoriser l’accès chez ta banque.', action: 'Autoriser ma banque' }
    default:
      return null
  }
})

const syncing = ref(false)
async function syncNow() {
  syncing.value = true
  try {
    await call(api.POST('/bank/sync'))
    toast.show('Synchronisation lancée. Les nouvelles opérations arrivent dans quelques secondes.')
    setTimeout(load, 5000)
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Synchronisation impossible.', 'error')
  } finally {
    syncing.value = false
  }
}

// DEV uniquement (npm run dev + APP_ENV=dev côté API) : reprend 30 jours d'historique.
const isDev = import.meta.env.DEV
async function devBackfill() {
  if (!confirm('DEV : effacer les opérations importées et reprendre les 30 derniers jours ?')) return
  syncing.value = true
  try {
    const res = await fetch('/api/v1/dev/backfill', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'epargne' },
      body: JSON.stringify({ days: 30 }),
    })
    const body = await res.json()
    if (!res.ok) throw new Error(body.detail ?? `HTTP ${res.status}`)
    toast.show(`${body.inserted} opérations importées, ${body.rounded} arrondies.`)
    await load()
  } catch (e) {
    toast.show(e instanceof Error ? e.message : 'Reprise impossible.', 'error')
  } finally {
    syncing.value = false
  }
}

const accountSheet = ref(false)
async function track(id: string) {
  try {
    await call(api.PUT('/bank/account', { body: { account_id: id } }))
    accountSheet.value = false
    await load()
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Compte non modifié.', 'error')
  }
}

const disconnectSheet = ref(false)
const disconnecting = ref(false)
async function disconnect() {
  disconnecting.value = true
  try {
    await call(api.DELETE('/bank/connection'))
    disconnectSheet.value = false
    conn.value = null
    step.value = 1
    toast.show('Banque déconnectée. Tes opérations et arrondis ont été supprimés.')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Déconnexion impossible.', 'error')
  } finally {
    disconnecting.value = false
  }
}

// --- assistant ---
const step = ref(1)
const consent = ref(false)
const appId = ref('')
const pem = ref('')
const pemName = ref('')
const bankCode = ref<string>('boursobank')
const submitting = ref(false)
const wizardError = ref<string | null>(null)
const editingCredentials = ref(false)
const showWizard = computed(() => loaded.value && !loadError.value && (!conn.value || editingCredentials.value))

async function readPem(ev: Event) {
  const file = (ev.target as HTMLInputElement).files?.[0]
  if (!file) return
  pem.value = await file.text()
  pemName.value = file.name
}
const pemLooksValid = computed(() => /-----BEGIN (RSA )?PRIVATE KEY-----/.test(pem.value))

async function authorize() {
  submitting.value = true
  wizardError.value = null
  try {
    await call(api.PUT('/bank/credentials', { body: { bank_code: bankCode.value as 'boursobank', eb_application_id: appId.value.trim(), private_key_pem: pem.value } }))
    const { url } = await call(api.POST('/bank/authorizations'))
    pem.value = '' // la clé ne reste pas dans la page ; gardée jusque-là pour pouvoir réessayer
    window.location.assign(url)
  } catch (e) {
    wizardError.value = e instanceof ApiError ? e.detail : 'Impossible de joindre le serveur.'
    submitting.value = false
  }
}

function editCredentials() {
  editingCredentials.value = true
  step.value = 2
  if (conn.value) bankCode.value = conn.value.bank_code
  consent.value = true
}
function goBack() {
  if (editingCredentials.value && step.value === 2) editingCredentials.value = false
  else if (step.value > 1) step.value--
  else router.push('/reglages')
}
</script>

<template>
  <PageTitle title="Ma banque" back="/reglages" />
  <div class="lg:flex lg:items-start lg:gap-6">
  <SettingsNav />
  <div class="min-w-0 flex-1">

  <div v-if="returned" class="card mx-4 lg:mx-0 mb-4" :class="returned.cls" role="status">{{ returned.text }}</div>
  <div v-if="loadError" class="card mx-4 lg:mx-0 bg-danger-soft text-danger" role="alert">{{ loadError }}</div>
  <p v-else-if="!loaded" class="px-4 text-ink-muted" aria-live="polite">Chargement…</p>

  <!-- Assistant -->
  <section v-else-if="showWizard" class="mx-4 flex flex-col gap-4 lg:mx-0 lg:max-w-[720px] lg:rounded-[var(--radius-md)] lg:border lg:border-surface-sunken lg:bg-surface-raised lg:p-6">
    <div class="grid grid-cols-4 gap-2" aria-hidden="true">
      <span v-for="i in 4" :key="i" class="h-1.5 rounded-full" :class="i <= step ? 'bg-primary' : 'bg-surface-sunken'" />
    </div>
    <p class="text-label text-ink-muted">Étape {{ step }} sur 4</p>

    <template v-if="step === 1">
      <h2 class="text-heading">Ce que lit Arrondi</h2>
      <div class="card flex flex-col gap-2">
        <p>Arrondi lit tes opérations en <strong>lecture seule</strong> : il ne peut jamais initier de paiement ni de virement.</p>
        <ul class="list-disc pl-5 text-ink-muted">
          <li>les opérations de ton compte courant (date, montant, libellé) ;</li>
          <li>les quatre derniers caractères de ton IBAN ;</li>
          <li>le nom de tes comptes.</li>
        </ul>
        <p class="text-ink-muted">Tu peux tout exporter ou tout supprimer depuis « Compte ».</p>
      </div>
      <label class="flex min-h-11 items-start gap-3">
        <input v-model="consent" type="checkbox" class="mt-1 size-5 shrink-0 accent-[var(--primary)]" />
        <span>J’accepte qu’Arrondi lise ces données pour calculer mes arrondis.</span>
      </label>
      <button class="btn-primary" :disabled="!consent" @click="step = 2">Continuer</button>
    </template>

    <template v-else-if="step === 2">
      <h2 class="text-heading">Ton application Enable Banking</h2>
      <p class="text-ink-muted">
        Sur enablebanking.com, crée une application de production, lie ton compte bancaire, puis récupère son identifiant
        et le fichier <code>.pem</code> téléchargé à la création.
      </p>
      <TextField id="app-id" v-model="appId" label="Identifiant de l’application" autocomplete="off" spellcheck="false" placeholder="a0def487-…" />
      <div class="flex flex-col gap-1">
        <span class="text-label text-ink-muted">Clé privée (.pem)</span>
        <label class="btn-secondary cursor-pointer">
          <input type="file" accept=".pem,application/x-pem-file,text/plain" class="sr-only" @change="readPem" />
          {{ pemName ? `Fichier : ${pemName}` : 'Choisir le fichier .pem' }}
        </label>
        <details class="text-caption text-ink-muted">
          <summary class="min-h-11 cursor-pointer py-3">Coller la clé à la place</summary>
          <textarea
            v-model="pem"
            rows="5"
            spellcheck="false"
            class="w-full rounded-[var(--radius-sm)] border border-line bg-surface-raised p-3 font-mono text-caption text-ink"
            placeholder="-----BEGIN PRIVATE KEY-----"
          />
        </details>
        <span class="text-caption" :class="pem && !pemLooksValid ? 'text-danger' : 'text-ink-muted'">
          {{ pem && !pemLooksValid ? 'Ce fichier ne ressemble pas à une clé privée.' : 'La clé est chiffrée sur le serveur et n’est jamais réaffichée.' }}
        </span>
      </div>
      <div class="flex gap-3">
        <button class="btn-secondary flex-1" @click="goBack">Retour</button>
        <button class="btn-primary flex-1" :disabled="!appId.trim() || !pemLooksValid" @click="step = 3">Continuer</button>
      </div>
    </template>

    <template v-else-if="step === 3">
      <h2 class="text-heading">Choisis ta banque</h2>
      <ul class="flex flex-col gap-3" role="radiogroup" aria-label="Banque">
        <li v-for="b in banks" :key="b.code">
          <button
            role="radio"
            :aria-checked="bankCode === b.code"
            :disabled="!b.available || (editingCredentials && b.code !== conn?.bank_code)"
            class="flex min-h-14 w-full items-center justify-between rounded-[var(--radius-md)] border px-4 text-left disabled:text-ink-muted"
            :class="bankCode === b.code ? 'border-primary bg-primary-soft' : 'border-surface-sunken bg-surface-raised'"
            @click="bankCode = b.code"
          >
            <span class="text-heading" :class="b.available ? '' : 'font-normal'">{{ b.name }}</span>
            <span v-if="bankCode === b.code" class="text-primary" aria-hidden="true">✓</span>
            <span v-else-if="!b.available" class="pill bg-surface-sunken text-ink-muted">Bientôt disponible</span>
          </button>
        </li>
      </ul>
      <p class="text-caption text-ink-muted">
        Une banque devient disponible quand son profil a été validé sur un compte réel. Pour Crédit Agricole, tu choisiras
        ensuite ta caisse régionale.
      </p>
      <div class="flex gap-3">
        <button class="btn-secondary flex-1" @click="goBack">Retour</button>
        <button class="btn-primary flex-1" @click="step = 4">Continuer</button>
      </div>
    </template>

    <template v-else>
      <h2 class="text-heading">Autorise l’accès</h2>
      <p>
        Tu vas être redirigé vers <strong>{{ bankName(bankCode) }}</strong> pour autoriser Arrondi. Valide dans l’application de
        ta banque si elle te le demande, puis tu reviendras ici.
      </p>
      <p class="text-ink-muted">L’autorisation dure jusqu’à 180 jours ; Arrondi te préviendra avant son expiration.</p>
      <div v-if="wizardError" class="card bg-danger-soft text-danger" role="alert">{{ wizardError }}</div>
      <div class="flex gap-3">
        <button class="btn-secondary flex-1" :disabled="submitting" @click="goBack">Retour</button>
        <button class="btn-primary flex-1" :disabled="submitting" @click="authorize">{{ submitting ? 'Redirection…' : 'Autoriser' }}</button>
      </div>
    </template>
  </section>

  <!-- Connexion existante -->
  <section v-else-if="conn" class="mx-4 lg:mx-0 flex flex-col gap-6">
    <StatusBanner v-if="banner" :tone="banner.tone" :text="banner.text" :action="banner.action" :busy="reconnecting" @act="reconnect" />

    <div class="card flex flex-col p-0">
      <div class="flex items-center justify-between gap-2 px-4 pt-4 pb-2">
        <h2 class="text-heading">{{ bankName(conn.bank_code) }}</h2>
        <span class="pill" :class="status.cls">{{ status.label }}</span>
      </div>
      <ul class="divide-y divide-surface-sunken px-4">
        <li>
          <button class="flex min-h-16 w-full items-center justify-between gap-2 text-left" :disabled="!conn.accounts?.length" @click="accountSheet = true">
            <span>
              <span class="block">Compte suivi</span>
              <span class="text-caption text-ink-muted">
                {{ tracked ? `${tracked.label ?? 'Compte'}${tracked.iban_last4 ? ` ····${tracked.iban_last4}` : ''}` : 'Aucun : choisis-en un' }}
              </span>
            </span>
            <AppIcon v-if="conn.accounts?.length" name="chevron" class="text-ink-muted" />
          </button>
        </li>
        <li v-if="conn.consent_valid_until" class="flex min-h-16 items-center justify-between gap-2">
          <span>
            <span class="block">Fin du consentement</span>
            <span class="text-caption text-ink-muted">Dans {{ conn.expires_in_days }} jours</span>
          </span>
          <span class="font-semibold">{{ formatDate(conn.consent_valid_until) }}</span>
        </li>
        <li class="flex min-h-16 items-center justify-between gap-2">
          <span>Dernière synchronisation</span>
          <span class="text-right text-ink-muted">{{ conn.last_sync_ok_at ? formatSyncTime(conn.last_sync_ok_at) : 'Jamais' }}</span>
        </li>
      </ul>
    </div>

    <div class="flex flex-col gap-3 lg:flex-row lg:flex-wrap lg:items-center">
    <button v-if="conn.status === 'active' || conn.status === 'expiring_soon'" class="btn-secondary" :disabled="syncing || !tracked" @click="syncNow">
      <AppIcon name="sync" :size="20" class="mr-2" />{{ syncing ? 'Lancement…' : 'Synchroniser maintenant' }}
    </button>
    <button v-if="isDev && conn.status === 'active'" class="btn-secondary" :disabled="syncing || !tracked" @click="devBackfill">
      DEV : reprendre 30 jours d’historique
    </button>
    <button class="btn-secondary" @click="editCredentials">Modifier mes identifiants Enable Banking</button>
    <button class="link-danger lg:ml-auto" @click="disconnectSheet = true">Déconnecter</button>
    </div>

    <section v-if="isDesktop && runs.length" class="card p-2">
      <table class="w-full border-collapse">
        <caption class="px-3 pt-3 pb-1 text-left text-heading">Dernières synchronisations</caption>
        <thead>
          <tr class="border-b border-line text-left text-label text-ink-muted">
            <th scope="col" class="px-3 py-2.5">Quand</th>
            <th scope="col" class="px-3 py-2.5">Déclenchement</th>
            <th scope="col" class="px-3 py-2.5">Résultat</th>
            <th scope="col" class="px-3 py-2.5 text-right">Statut</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in runs" :key="r.started_at" class="border-b border-surface-sunken last:border-0">
            <td class="px-3 py-3 first-letter:uppercase">{{ formatSyncTime(r.started_at) }}</td>
            <td class="px-3 py-3 text-ink-muted">{{ triggerLabel(r.trigger) }}</td>
            <td class="px-3 py-3">{{ runResult(r) }}</td>
            <td class="px-3 py-3 text-right">
              <span class="pill" :class="r.status === 'success' ? 'bg-primary-soft text-ink' : r.status === 'error' ? 'bg-warning-soft text-warning' : 'bg-surface-sunken text-ink-muted'">
                {{ r.status === 'success' ? 'Réussie' : r.status === 'error' ? 'Échec' : 'En cours' }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </section>
    <p class="text-caption text-ink-muted">Lecture seule : l’application ne peut jamais initier de paiement. Synchronisation manuelle : 1 par heure, 4 sur 24 heures.</p>
  </section>
  </div>
  </div>

  <BottomSheet :open="accountSheet" title="Compte suivi" @close="accountSheet = false">
    <p class="mb-3 text-ink-muted">Les arrondis sont calculés sur les paiements par carte de ce compte.</p>
    <ul v-if="conn" class="flex flex-col gap-2">
      <li v-for="a in conn.accounts ?? []" :key="a.id">
        <button
          class="flex min-h-14 w-full items-center justify-between rounded-[var(--radius-md)] border px-4 text-left"
          :class="a.is_tracked ? 'border-primary bg-primary-soft' : 'border-surface-sunken'"
          @click="track(a.id)"
        >
          <span>{{ a.label ?? 'Compte' }}<span v-if="a.iban_last4" class="text-ink-muted"> ····{{ a.iban_last4 }}</span></span>
          <span v-if="a.is_tracked" class="text-primary" aria-label="suivi">✓</span>
        </button>
      </li>
    </ul>
  </BottomSheet>

  <BottomSheet :open="disconnectSheet" title="Déconnecter la banque ?" @close="disconnectSheet = false">
    <p class="mb-4">
      La connexion, tes opérations et tous tes arrondis seront <strong>supprimés</strong>. Tes virements et ton objectif sont
      conservés. Tu pourras reconnecter ta banque ensuite, mais le comptage repartira de zéro.
    </p>
    <button class="btn-danger w-full" :disabled="disconnecting" @click="disconnect">{{ disconnecting ? 'Suppression…' : 'Déconnecter et supprimer' }}</button>
  </BottomSheet>
</template>
