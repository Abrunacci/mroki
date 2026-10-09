import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import RequestDetail from './RequestDetail.vue'
import type { FieldConversion, Gate, RequestDetail as RequestDetailType, Response } from '@/api'

const getGate = vi.fn()
const getRequest = vi.fn()
const updateGate = vi.fn()
// Mock the underlying API modules so the real query adapters keep working while
// the network is stubbed.
vi.mock('@/api/gates', () => ({
  getGate: (...a: unknown[]) => getGate(...a),
  updateGate: (...a: unknown[]) => updateGate(...a),
}))
vi.mock('@/api/requests', () => ({
  getRequest: (...a: unknown[]) => getRequest(...a),
}))

const push = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'gate-1', rid: 'req-1' } }),
  useRouter: () => ({ push }),
}))

// A fresh, retry-free client per mount isolates the gate/request cache per test
// and keeps error-path mutations (e.g. update failure) from retrying.
function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function makeGate(overrides: Partial<Gate> = {}): Gate {
  return {
    id: 'gate-1',
    name: 'checkout',
    live_url: 'https://live.example.com',
    shadow_url: 'https://shadow.example.com',
    diff_config: {
      ignored_fields: [],
      included_fields: [],
      float_tolerance: 0,
      sort_arrays: false,
    },
    redacted_fields: [],
    retention: '',
    shadow_adapter: null,
    created_at: '2026-07-30T09:00:00Z',
    stats: { request_count_24h: 0, diff_count_24h: 0, diff_rate: 0, last_active: null },
    ...overrides,
  }
}

function makeResponse(overrides: Partial<Response> = {}): Response {
  return {
    id: 'resp-1',
    status_code: 200,
    headers: {},
    body: null,
    latency_ms: 10,
    created_at: '2026-07-30T09:00:00Z',
    ...overrides,
  }
}

function makeRequest(overrides: Partial<RequestDetailType> = {}): RequestDetailType {
  return {
    id: 'req-1',
    method: 'POST',
    path: '/api/orders',
    headers: { 'Content-Type': ['application/json'] },
    body: '{"a":1}',
    created_at: '2026-07-30T09:00:00Z',
    live_response: makeResponse(),
    shadow_response: makeResponse({ id: 'resp-2', latency_ms: 12 }),
    diff: {
      content: [],
      config: { ignored_fields: [], included_fields: [], float_tolerance: 0, sort_arrays: false },
      shadow_adapter: null,
    },
    ...overrides,
  }
}

// Slot-rendering stubs so the dropdown items (and their click handlers) mount.
const passthrough = { template: '<div><slot /></div>' }
const global = {
  stubs: {
    DiffViewer: {
      name: 'DiffViewer',
      props: [
        'liveResponse',
        'shadowResponse',
        'diffContent',
        'diffConfig',
        'bodyOnly',
        'conversions',
      ],
      template: '<div />',
    },
    Alert: passthrough,
    AlertTitle: passthrough,
    AlertDescription: passthrough,
    Button: { template: '<button><slot /></button>' },
    DropdownMenu: passthrough,
    DropdownMenuTrigger: passthrough,
    DropdownMenuContent: passthrough,
    // No inner @click: the parent binds @click on the item, so a native click
    // on the stub's root reaches that listener exactly once.
    DropdownMenuItem: { name: 'DropdownMenuItem', template: '<div><slot /></div>' },
    TooltipProvider: passthrough,
    Tooltip: passthrough,
    TooltipTrigger: passthrough,
    TooltipContent: passthrough,
  },
}

async function mountDetail(request = makeRequest(), gate = makeGate()) {
  getGate.mockResolvedValue({ data: gate })
  getRequest.mockResolvedValue({ data: request })
  const wrapper = mount(RequestDetail, {
    global: {
      ...global,
      plugins: [[VueQueryPlugin, { queryClient: makeQueryClient() }]],
    },
  })
  await flushPromises()
  return wrapper
}

describe('RequestDetail loading and error states', () => {
  beforeEach(() => {
    getGate.mockReset()
    getRequest.mockReset()
  })

  it('shows the loading state while either query is in flight', () => {
    // The gate resolves but the request never does, so the combined
    // loading = gatePending || requestPending stays true.
    getGate.mockResolvedValue({ data: makeGate() })
    getRequest.mockReturnValue(new Promise(() => {}))
    const wrapper = mount(RequestDetail, {
      global: { ...global, plugins: [[VueQueryPlugin, { queryClient: makeQueryClient() }]] },
    })

    expect(wrapper.text()).toContain('Loading request...')
    expect(wrapper.findComponent({ name: 'DiffViewer' }).exists()).toBe(false)
  })

  it('surfaces the request error message when the request fetch rejects', async () => {
    getGate.mockResolvedValue({ data: makeGate() })
    getRequest.mockRejectedValue(new Error('request boom'))
    const wrapper = mount(RequestDetail, {
      global: { ...global, plugins: [[VueQueryPlugin, { queryClient: makeQueryClient() }]] },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Error')
    expect(wrapper.text()).toContain('request boom')
    expect(wrapper.findComponent({ name: 'DiffViewer' }).exists()).toBe(false)
  })

  it('falls back to the gate error message when only the gate fetch rejects', async () => {
    getGate.mockRejectedValue(new Error('gate boom'))
    getRequest.mockResolvedValue({ data: makeRequest() })
    const wrapper = mount(RequestDetail, {
      global: { ...global, plugins: [[VueQueryPlugin, { queryClient: makeQueryClient() }]] },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('gate boom')
  })
})

describe('RequestDetail copy cURL', () => {
  const writeText = vi.fn()
  beforeEach(() => {
    getGate.mockReset()
    getRequest.mockReset()
    updateGate.mockReset()
    writeText.mockReset().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('writes a curl command for the chosen endpoint to the clipboard', async () => {
    const wrapper = await mountDetail()
    const items = wrapper.findAllComponents({ name: 'DropdownMenuItem' })
    const live = items.find((i) => i.text().trim() === 'Live endpoint')!
    await live.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledTimes(1)
    const curl = writeText.mock.calls[0][0] as string
    expect(curl).toContain("curl -X POST 'https://live.example.com/api/orders'")
    expect(curl).toContain("-H 'Content-Type: application/json'")
    expect(curl).toContain('-d \'{"a":1}\'')
  })
})

describe('RequestDetail export JSON', () => {
  beforeEach(() => {
    getGate.mockReset()
    getRequest.mockReset()
  })

  it('serializes the request into a downloadable JSON blob', async () => {
    const createObjectURL = vi.fn(() => 'blob:mock')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL })
    const click = vi.fn()
    const anchor = { href: '', download: '', click } as unknown as HTMLAnchorElement
    // Only intercept the download anchor; let every other tag render normally.
    const realCreateElement = document.createElement.bind(document)
    const createElement = vi
      .spyOn(document, 'createElement')
      .mockImplementation((tag: string) => (tag === 'a' ? anchor : realCreateElement(tag)))

    const wrapper = await mountDetail()
    const exportBtn = wrapper.findAll('button').find((b) => b.text().includes('Export JSON'))!
    await exportBtn.trigger('click')

    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(anchor.download).toBe('request-req-1.json')
    expect(click).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')

    createElement.mockRestore()
    vi.unstubAllGlobals()
  })
})

describe('RequestDetail incomplete data', () => {
  beforeEach(() => {
    getGate.mockReset()
    getRequest.mockReset()
  })

  it('warns when a response is missing instead of rendering the diff', async () => {
    const req = makeRequest({ shadow_response: null as unknown as Response })
    const wrapper = await mountDetail(req)
    expect(wrapper.text()).toContain('Incomplete Data')
    expect(wrapper.text()).toContain('missing shadow response data')
    expect(wrapper.findComponent({ name: 'DiffViewer' }).exists()).toBe(false)
  })
})

describe('RequestDetail ignore field', () => {
  beforeEach(() => {
    getGate.mockReset()
    getRequest.mockReset()
    updateGate.mockReset()
  })
  afterEach(() => vi.useRealTimers())

  it('persists the ignored field and shows a transient success toast', async () => {
    const updated = makeGate({
      diff_config: { ...makeGate().diff_config, ignored_fields: ['body.id'] },
    })
    updateGate.mockResolvedValue({ data: updated })
    vi.useFakeTimers()
    const wrapper = await mountDetail()

    wrapper.findComponent({ name: 'DiffViewer' }).vm.$emit('ignore-field', 'body.id')
    await flushPromises()

    expect(updateGate).toHaveBeenCalledWith('gate-1', {
      diff_config: expect.objectContaining({ ignored_fields: ['body.id'] }),
    })
    expect(wrapper.text()).toContain('Ignoring "body.id" in future diffs')

    // The toast auto-dismisses after 3s.
    vi.advanceTimersByTime(3000)
    await flushPromises()
    expect(wrapper.text()).not.toContain('Ignoring "body.id" in future diffs')
  })

  it('shows an error toast when the update fails', async () => {
    updateGate.mockRejectedValue(new Error('conflict'))
    const wrapper = await mountDetail()

    wrapper.findComponent({ name: 'DiffViewer' }).vm.$emit('ignore-field', 'body.id')
    await flushPromises()

    expect(wrapper.text()).toContain('conflict')
  })
})

describe('RequestDetail shadow adapter', () => {
  beforeEach(() => {
    getGate.mockReset()
    getRequest.mockReset()
  })

  const adapter = { type: 'graphql', config: 'endpoint: /graphql', version: 'bbbbbbbbbbbb' }

  function adapterRequest(version: string, conversions: FieldConversion[] = []) {
    const base = makeRequest()
    return makeRequest({
      method: 'GET',
      path: '/bookings/1042',
      diff: { ...base.diff, shadow_adapter: { type: 'graphql', version, conversions } },
    })
  }

  it('lists the declared conversions with the values the shadow sent', async () => {
    const conversions: FieldConversion[] = [
      { field: 'id', as: 'number', original: '1042' },
      { field: 'check_in', as: 'date', original: '2026-10-09T00:00:00-03:00', error: 'not UTC' },
    ]
    const wrapper = await mountDetail(
      adapterRequest('bbbbbbbbbbbb', conversions),
      makeGate({ shadow_adapter: adapter })
    )

    const summary = wrapper.find('[data-testid="conversions-summary"]')
    expect(summary.text()).toContain('2 fields compared with a type conversion')
    expect(summary.text()).toContain('id as number · sent "1042"')
    expect(summary.text()).toContain('check_in as date failed: not UTC')
    expect(wrapper.findComponent({ name: 'DiffViewer' }).props('conversions')).toEqual(conversions)
  })

  it('shows no conversion summary when the mapping declares none', async () => {
    const wrapper = await mountDetail(
      adapterRequest('bbbbbbbbbbbb'),
      makeGate({ shadow_adapter: adapter })
    )
    expect(wrapper.find('[data-testid="conversions-summary"]').exists()).toBe(false)
  })

  it('explains a body-only REST → GraphQL comparison and its mapping version', async () => {
    const wrapper = await mountDetail(
      adapterRequest('bbbbbbbbbbbb'),
      makeGate({ shadow_adapter: adapter })
    )

    const note = wrapper.find('[data-testid="shadow-adapter-note"]')
    expect(note.exists()).toBe(true)
    expect(note.text()).toContain('REST → GraphQL')
    expect(note.text()).toContain('bbbbbbbbbbbb')
    expect(wrapper.text()).toContain('(not compared)')
    expect(wrapper.find('[data-testid="mapping-changed-warning"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'DiffViewer' }).props('bodyOnly')).toBe(true)
    // Replaying the REST request against the GraphQL shadow makes no sense.
    expect(wrapper.text()).not.toContain('Shadow endpoint')
  })

  it('warns when the request was compared with another mapping', async () => {
    const wrapper = await mountDetail(
      adapterRequest('aaaaaaaaaaaa'),
      makeGate({ shadow_adapter: adapter })
    )

    const warning = wrapper.find('[data-testid="mapping-changed-warning"]')
    expect(warning.exists()).toBe(true)
    expect(warning.text()).toContain('aaaaaaaaaaaa')
    expect(warning.text()).toContain('bbbbbbbbbbbb')
  })

  it('warns when the gate no longer has a mapping', async () => {
    const wrapper = await mountDetail(adapterRequest('aaaaaaaaaaaa'), makeGate())

    expect(wrapper.find('[data-testid="mapping-changed-warning"]').text()).toContain(
      'no longer has a mapping'
    )
  })

  it('shows none of it for regular comparisons', async () => {
    const wrapper = await mountDetail()

    expect(wrapper.find('[data-testid="shadow-adapter-note"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('(not compared)')
    expect(wrapper.text()).toContain('Shadow endpoint')
    expect(wrapper.findComponent({ name: 'DiffViewer' }).props('bodyOnly')).toBe(false)
  })
})
