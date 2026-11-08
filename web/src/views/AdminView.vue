<script setup lang="ts">
// E-08 Administration : membres, activation, état de connexion (jamais leurs données financières).
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { bankStatus } from '@/lib/bankStatus'
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

const statusOf = (u: U) => bankStatus[u.connection_status ?? 'none'] ?? bankStatus.none!
</script>

<template>
  <PageTitle title="Administration" back="/reglages" />
  <div v-if="error" class="card mx-4 bg-danger-soft text-danger" role="alert">{{ error }}</div>

  <template v-else-if="users">
    <ul class="mx-4 divide-y divide-surface-sunken">
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
    <div class="mx-4 mt-6 flex flex-col gap-3">
      <button class="btn-primary" @click="sheet = true">Ajouter un utilisateur</button>
      <p class="text-caption text-ink-muted">Tu vois l’état de connexion de chaque utilisateur, jamais ses données financières.</p>
    </div>
  </template>
  <p v-else class="px-4 text-ink-muted" aria-live="polite">Chargement…</p>

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
