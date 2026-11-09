<script setup lang="ts">
// E-08 Administration : membres, activation, état de connexion (jamais leurs données financières).
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { bankStatus } from '@/lib/bankStatus'
import { formatSyncTime } from '@/lib/format'
import { useDesktop } from '@/lib/useDesktop'
import { useSession } from '@/stores/session'
import { useToast } from '@/stores/toast'
import BottomSheet from '@/components/BottomSheet.vue'
import PageTitle from '@/components/PageTitle.vue'
import TextField from '@/components/TextField.vue'
import ToggleSwitch from '@/components/ToggleSwitch.vue'

type U = Schemas['AdminUserOut']

const session = useSession()
const router = useRouter()
const toast = useToast()
const users = ref<U[] | null>(null)
const error = ref<string | null>(null)

async function load() {
  try {
    users.value = await call(api.GET('/admin/users'))
  } catch (e) {
    error.value = e instanceof ApiError ? e.detail : 'Impossible de joindre le serveur.'
  }
}
onMounted(() => (session.isAdmin ? load() : router.replace('/reglages')))

async function setActive(u: U, active: boolean) {
  const before = u.is_active
  u.is_active = active
  try {
    await call(api.PATCH('/admin/users/{id}', { params: { path: { id: u.id } }, body: { is_active: active } }))
  } catch (e) {
    u.is_active = before
    toast.show(e instanceof ApiError ? e.detail : 'Modification impossible.', 'error')
  }
}

const sheet = ref(false)
const email = ref('')
const displayName = ref('')
const formError = ref<string | null>(null)
const saving = ref(false)
async function invite() {
  formError.value = null
  saving.value = true
  try {
    await call(api.POST('/admin/users', { body: { email: email.value.trim().toLowerCase(), display_name: displayName.value.trim() } }))
    sheet.value = false
    toast.show(`${displayName.value.trim()} est invité. Ajoute aussi son email dans Cloudflare Access.`)
    email.value = displayName.value = ''
    await load()
  } catch (e) {
    formError.value = e instanceof ApiError ? e.detail : 'Invitation impossible.'
  } finally {
    saving.value = false
  }
}

const isDesktop = useDesktop()
const now = Date.now()
// « Expire bientôt » : moins de 14 jours (section 9.5), calculé depuis la fin de l'accès.
function effective(u: U) {
  const st = u.connection_status ?? 'none'
  if (st === 'active' && u.consent_valid_until && Date.parse(u.consent_valid_until) - now < 14 * 86400000) return 'expiring_soon'
  return st
}
const statusOf = (u: U) => bankStatus[effective(u)] ?? bankStatus.none!
// D-08 : compteurs, jamais de montants (D-12).
const counters = computed(() => {
  const us = users.value ?? []
  return [
    ['Membres actifs', `${us.filter((u) => u.is_active).length} sur ${us.length}`],
    ['Banques connectées', String(us.filter((u) => ['active', 'expiring_soon'].includes(effective(u))).length)],
    ['Accès à renouveler', String(us.filter((u) => ['expiring_soon', 'expired', 'error'].includes(effective(u))).length)],
    ['Sans banque', String(us.filter((u) => effective(u) === 'none').length)],
  ]
})
</script>

<template>
  <template v-if="isDesktop">
    <PageTitle title="Administration" subtitle="Les membres de la famille et l’état de leur connexion bancaire." />
    <div v-if="error" class="card bg-danger-soft text-danger" role="alert">{{ error }}</div>
    <template v-else-if="users">
      <div class="mb-4 grid grid-cols-2 gap-4 xl:grid-cols-4">
        <div v-for="[k, v] in counters" :key="k" class="card flex flex-col gap-1 p-4">
          <span class="text-label text-ink-muted">{{ k }}</span>
          <span class="text-title">{{ v }}</span>
        </div>
      </div>
      <div class="flex flex-col items-stretch gap-4 xl:flex-row xl:items-start">
        <section class="card min-w-0 flex-1 overflow-x-auto p-2">
          <table class="w-full border-collapse">
            <caption class="px-3 pt-3 pb-1 text-left text-heading">Membres</caption>
            <thead>
              <tr class="border-b border-line text-left text-label text-ink-muted">
                <th scope="col" class="px-3 py-2.5">Membre</th>
                <th scope="col" class="px-3 py-2.5">Rôle</th>
                <th scope="col" class="px-3 py-2.5">Banque</th>
                <th scope="col" class="px-3 py-2.5">Dernière synchro</th>
                <th scope="col" class="px-3 py-2.5 text-right">Actif</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="u in users" :key="u.id" class="border-b border-surface-sunken last:border-0" :class="u.is_active ? '' : 'text-ink-muted'">
                <td class="max-w-64 px-3 py-3">
                  <span class="block truncate font-semibold">{{ u.display_name }}</span>
                  <span class="block truncate text-caption text-ink-muted">{{ u.email }}</span>
                </td>
                <td class="px-3 py-3 text-caption text-ink-muted">{{ u.role === 'admin' ? 'Administrateur' : 'Membre' }}</td>
                <td class="px-3 py-3"><span class="pill whitespace-nowrap" :class="statusOf(u).cls">{{ statusOf(u).label }}</span></td>
                <td class="px-3 py-3 text-caption whitespace-nowrap text-ink-muted">{{ u.last_sync_ok_at ? formatSyncTime(u.last_sync_ok_at) : '–' }}</td>
                <td class="px-3 py-3 text-right">
                  <ToggleSwitch
                    :model-value="u.is_active"
                    :label="`Accès de ${u.display_name}`"
                    :disabled="u.id === session.me?.id"
                    @update:model-value="(v) => setActive(u, v)"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </section>
        <section class="card flex shrink-0 flex-col gap-4 p-5 xl:w-80">
          <h2 class="text-heading">Ajouter un membre</h2>
          <form class="flex flex-col gap-4" @submit.prevent="invite">
            <TextField id="invd-name" v-model="displayName" label="Prénom et initiale" maxlength="60" placeholder="Hugo M." />
            <TextField id="invd-email" v-model="email" label="Email" type="email" autocomplete="off" placeholder="hugo@exemple.fr" :error="formError" />
            <button type="submit" class="btn-primary" :disabled="saving || !email.trim() || !displayName.trim()">{{ saving ? 'Ajout…' : 'Ajouter' }}</button>
            <p class="text-caption text-ink-muted">Ajoute aussi son email dans la liste autorisée de Cloudflare Access. Tu ne vois jamais les montants d’un membre.</p>
          </form>
        </section>
      </div>
    </template>
    <p v-else class="text-ink-muted" aria-live="polite">Chargement…</p>
  </template>

  <template v-else>
  <PageTitle title="Administration" back="/reglages" />
  <div v-if="error" class="card mx-4 lg:mx-0 bg-danger-soft text-danger" role="alert">{{ error }}</div>

  <template v-else-if="users">
    <ul class="mx-4 lg:mx-0 divide-y divide-surface-sunken">
      <li v-for="u in users" :key="u.id" class="flex items-center justify-between gap-3 py-3">
        <div class="min-w-0" :class="u.is_active ? '' : 'opacity-60'">
          <p class="truncate text-heading">{{ u.display_name }} · {{ u.role === 'admin' ? 'Administrateur' : 'Membre' }}</p>
          <p class="truncate text-ink-muted">{{ u.email }}</p>
          <span class="pill mt-1" :class="statusOf(u).cls">{{ statusOf(u).label }}</span>
        </div>
        <ToggleSwitch
          :model-value="u.is_active"
          :label="`Accès de ${u.display_name}`"
          :disabled="u.id === session.me?.id"
          @update:model-value="(v) => setActive(u, v)"
        />
      </li>
    </ul>
    <div class="mx-4 lg:mx-0 mt-6 flex flex-col gap-3">
      <button class="btn-primary" @click="sheet = true">Ajouter un utilisateur</button>
      <p class="text-caption text-ink-muted">Tu vois l’état de connexion de chaque utilisateur, jamais ses données financières.</p>
    </div>
  </template>
  <p v-else class="px-4 text-ink-muted" aria-live="polite">Chargement…</p>
  </template>

  <BottomSheet :open="sheet" title="Ajouter un utilisateur" @close="sheet = false">
    <form class="flex flex-col gap-4" @submit.prevent="invite">
      <TextField id="inv-name" v-model="displayName" label="Prénom et initiale" maxlength="60" placeholder="Hugo M." />
      <TextField id="inv-email" v-model="email" label="Email" type="email" autocomplete="off" placeholder="hugo@exemple.fr" :error="formError" />
      <p class="text-caption text-ink-muted">
        Il devra aussi figurer dans la liste des emails autorisés de Cloudflare Access, puis créer sa propre application Enable Banking.
      </p>
      <button type="submit" class="btn-primary" :disabled="saving || !email.trim() || !displayName.trim()">{{ saving ? 'Ajout…' : 'Ajouter' }}</button>
    </form>
  </BottomSheet>
</template>
