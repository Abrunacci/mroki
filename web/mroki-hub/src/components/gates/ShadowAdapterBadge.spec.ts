import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ShadowAdapterBadge from './ShadowAdapterBadge.vue'

describe('ShadowAdapterBadge', () => {
  it('shows what the gate compares', () => {
    const wrapper = mount(ShadowAdapterBadge, { props: { type: 'graphql' } })

    expect(wrapper.text()).toContain('REST → GraphQL')
    expect(wrapper.find('code').exists()).toBe(false)
  })

  it('shows the mapping version when given', () => {
    const wrapper = mount(ShadowAdapterBadge, {
      props: { type: 'graphql', version: '3f2a9c41d07b' },
    })

    expect(wrapper.find('code').text()).toBe('3f2a9c41d07b')
    expect(wrapper.attributes('title')).toBe('Mapping version 3f2a9c41d07b')
  })
})
