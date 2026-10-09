import { describe, it, expect } from 'vitest'
import {
  conversionCounts,
  conversionNote,
  conversionPointer,
  conversionsByPointer,
  formatOriginal,
} from './conversions'

describe('conversionPointer', () => {
  it('points at the field under the body', () => {
    expect(conversionPointer('id')).toBe('/body/id')
    expect(conversionPointer('dates.check_in')).toBe('/body/dates/check_in')
  })

  it('escapes pointer characters', () => {
    expect(conversionPointer('a/b.c~d')).toBe('/body/a~1b/c~0d')
  })
})

describe('conversionsByPointer', () => {
  it('indexes by pointer and tolerates null', () => {
    const c = { field: 'id', as: 'number' as const, original: '1042' }
    expect(conversionsByPointer([c]).get('/body/id')).toBe(c)
    expect(conversionsByPointer(null).size).toBe(0)
  })
})

describe('conversionNote', () => {
  it('shows the type and the value the shadow sent', () => {
    expect(conversionNote({ field: 'id', as: 'number', original: '1042' })).toBe(
      'as number · sent "1042"'
    )
  })

  it('shows why a conversion failed', () => {
    expect(
      conversionNote({
        field: 'id',
        as: 'number',
        original: 'Qm9va2luZzoxMDQ0',
        error: '"Qm9va2luZzoxMDQ0" is not a number',
      })
    ).toBe('as number failed: "Qm9va2luZzoxMDQ0" is not a number')
  })
})

describe('formatOriginal', () => {
  it('renders JSON', () => {
    expect(formatOriginal('S')).toBe('"S"')
    expect(formatOriginal(1)).toBe('1')
    expect(formatOriginal(null)).toBe('null')
  })
})

describe('conversionCounts', () => {
  it('counts applied and failed conversions', () => {
    expect(
      conversionCounts([
        { field: 'id', as: 'number', original: '1' },
        { field: 'd', as: 'date', original: 'x', error: 'bad' },
      ])
    ).toEqual({ applied: 1, failed: 1 })
    expect(conversionCounts(undefined)).toEqual({ applied: 0, failed: 0 })
  })
})
