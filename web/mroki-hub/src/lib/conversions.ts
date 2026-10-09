import type { FieldConversion } from '@/api'

/**
 * JSON Pointer of a converted field in the diff view, where bodies live under
 * `/body`. `dates.check_in` -> `/body/dates/check_in` (RFC 6901 escapes).
 */
export function conversionPointer(field: string): string {
  return (
    '/body/' +
    field
      .split('.')
      .map((seg) => seg.replace(/~/g, '~0').replace(/\//g, '~1'))
      .join('/')
  )
}

/** Indexes conversions by the JSON Pointer of their field. */
export function conversionsByPointer(
  conversions: FieldConversion[] | null | undefined
): Map<string, FieldConversion> {
  return new Map((conversions ?? []).map((c) => [conversionPointer(c.field), c]))
}

/** The shadow value as received, as JSON text (e.g. `"1042"`). */
export function formatOriginal(original: unknown): string {
  return JSON.stringify(original) ?? 'null'
}

/**
 * Short note shown next to a converted value: what it was compared as and
 * what the shadow service sent, or why the conversion failed.
 */
export function conversionNote(c: FieldConversion): string {
  if (c.error) return `as ${c.as} failed: ${c.error}`
  return `as ${c.as} · sent ${formatOriginal(c.original)}`
}

/** Counts applied and failed conversions. */
export function conversionCounts(conversions: FieldConversion[] | null | undefined): {
  applied: number
  failed: number
} {
  const all = conversions ?? []
  const failed = all.filter((c) => c.error).length
  return { applied: all.length - failed, failed }
}
