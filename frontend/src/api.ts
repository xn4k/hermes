export type ApiErrorPayload = {
  error?: string
  details?: string
}

export class ApiError extends Error {
  readonly status: number
  readonly code?: string

  constructor(message: string, status: number, code?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

type ApiRequestOptions = RequestInit & {
  notifyUnauthorized?: boolean
}

let unauthorizedHandler: (() => void) | null = null

export function setUnauthorizedHandler(handler: (() => void) | null) {
  unauthorizedHandler = handler
}

function fallbackMessage(status: number) {
  if (status === 401) {
    return 'Deine Sitzung ist abgelaufen.'
  }

  if (status >= 500) {
    return 'Hermes hat gerade ein internes Problem.'
  }

  return `Die Anfrage ist fehlgeschlagen (HTTP ${status}).`
}

function messageForCode(code?: string) {
  const messages: Record<string, string> = {
    database_error: 'Die Datenbankanfrage ist fehlgeschlagen.',
    entry_not_found: 'Der Eintrag wurde nicht gefunden.',
    location_search_failed: 'Die Ortssuche ist gerade nicht erreichbar.',
    weather_fetch_failed: 'Die Wettermodelle konnten nicht geladen werden.',
    weather_settings_failed: 'Die Wettereinstellung konnte nicht geladen werden.',
    invalid_location: 'Der ausgewählte Ort ist ungültig.',
  }

  return code ? messages[code] : undefined
}

export async function apiRequest<T>(
  url: string,
  options: ApiRequestOptions = {},
): Promise<T> {
  const { notifyUnauthorized = true, ...requestOptions } = options
  const response = await fetch(url, {
    credentials: 'same-origin',
    ...requestOptions,
  })

  let payload: (T & ApiErrorPayload) | null = null

  try {
    payload = (await response.json()) as T & ApiErrorPayload
  } catch {
    // Einige Fehlerantworten enthalten keinen JSON-Body.
  }

  if (!response.ok) {
    if (response.status === 401 && notifyUnauthorized) {
      unauthorizedHandler?.()
    }

    throw new ApiError(
      messageForCode(payload?.error) ||
        payload?.details ||
        payload?.error ||
        fallbackMessage(response.status),
      response.status,
      payload?.error,
    )
  }

  if (payload === null) {
    throw new ApiError('Hermes hat eine ungültige Antwort gesendet.', response.status)
  }

  return payload
}
