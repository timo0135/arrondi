// Client de l'API, typé à partir du contrat OpenAPI (src/api/schema.d.ts,
// régénéré par `npm run gen:api` quand l'API change).
import createClient, { type Middleware } from 'openapi-fetch'
import type { components, paths } from './schema'

export type Schemas = components['schemas']

/** Erreur renvoyée par l'API (RFC 9457). */
export class ApiError extends Error {
  constructor(
    public status: number,
    public detail: string,
  ) {
    super(detail)
  }
}

// Section 10 : toute requête qui modifie des données porte X-Requested-With.
const csrf: Middleware = {
  onRequest({ request }) {
    if (!['GET', 'HEAD', 'OPTIONS'].includes(request.method)) {
      request.headers.set('X-Requested-With', 'epargne')
    }
    return request
  },
}

// Cloudflare Access gère la session : une 401 veut dire qu'elle a expiré.
// On recharge la page, Cloudflare redemande le code par email puis revient ici.
const sessionExpired: Middleware = {
  onResponse({ response }) {
    if (response.status === 401 && typeof window !== 'undefined' && !import.meta.env.DEV) {
      window.location.reload()
    }
    return response
  },
}

export const api = createClient<paths>({ baseUrl: '/api/v1', credentials: 'same-origin' })
api.use(csrf, sessionExpired)

/**
 * Déballe une réponse openapi-fetch : renvoie data, ou lève ApiError avec le
 * message de l'API (déjà rédigé en français pour l'utilisateur).
 */
export async function call<T>(p: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  const { data, error, response } = await p
  if (!response.ok) {
    const detail =
      (error as { detail?: string } | undefined)?.detail ??
      (response.status >= 500 ? 'Le service ne répond pas, réessaie dans un instant.' : 'Requête refusée.')
    throw new ApiError(response.status, detail)
  }
  return data as T
}
