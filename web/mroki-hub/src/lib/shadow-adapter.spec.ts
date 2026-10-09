import { describe, it, expect } from 'vitest'
import { shadowAdapterLabel } from './shadow-adapter'

describe('shadowAdapterLabel', () => {
  it('labels graphql as REST → GraphQL', () => {
    expect(shadowAdapterLabel('graphql')).toBe('REST → GraphQL')
  })

  it('falls back to the raw type', () => {
    expect(shadowAdapterLabel('soap')).toBe('soap')
  })
})
