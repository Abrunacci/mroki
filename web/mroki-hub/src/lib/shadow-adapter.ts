/**
 * Human-readable label for a shadow adapter type: what the gate compares
 * (live protocol → shadow protocol).
 */
export function shadowAdapterLabel(type: string): string {
  switch (type) {
    case 'graphql':
      return 'REST → GraphQL'
    default:
      return type
  }
}
