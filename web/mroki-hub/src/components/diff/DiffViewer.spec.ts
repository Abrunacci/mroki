import { describe, it, expect, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DiffViewer from './DiffViewer.vue'
import type { Response, PatchOp, DiffConfig } from '@/api'

function makeResponse(body: unknown, headers: Record<string, string[]> = {}): Response {
  return {
    id: 'resp-1',
    status_code: 200,
    headers,
    body: JSON.stringify(body),
    latency_ms: 10,
    created_at: '2026-07-30T09:00:00Z',
  }
}

function makeConfig(overrides: Partial<DiffConfig> = {}): DiffConfig {
  return {
    ignored_fields: [],
    included_fields: [],
    float_tolerance: 0,
    sort_arrays: false,
    ...overrides,
  }
}

// Stub the popover/tooltip internals so mounting does not require their runtime.
const global = {
  stubs: {
    Popover: true,
    PopoverContent: true,
    PopoverTrigger: true,
    TooltipProvider: false,
    Tooltip: false,
    TooltipContent: true,
    TooltipTrigger: false,
  },
}

interface MountArgs {
  live: unknown
  shadow: unknown
  content: PatchOp[]
  config?: Partial<DiffConfig>
  liveHeaders?: Record<string, string[]>
  shadowHeaders?: Record<string, string[]>
}

async function mountViewer(args: MountArgs) {
  const wrapper = mount(DiffViewer, {
    props: {
      liveResponse: makeResponse(args.live, args.liveHeaders),
      shadowResponse: makeResponse(args.shadow, args.shadowHeaders),
      diffContent: args.content,
      diffConfig: makeConfig(args.config),
    },
    global,
  })
  await flushPromises()
  // Switch to the patch list view where per-row ignore buttons live.
  const patchToggle = wrapper.findAll('button').find((b) => b.text() === 'Patch')
  if (patchToggle) {
    await patchToggle.trigger('click')
    await flushPromises()
  }
  return wrapper
}

describe('DiffViewer summary header', () => {
  it('counts body vs header changes and ignored fields', async () => {
    const wrapper = await mountViewer({
      live: { name: 'a' },
      shadow: { name: 'b' },
      liveHeaders: { 'X-Trace': ['1'] },
      shadowHeaders: { 'X-Trace': ['2'] },
      content: [
        { op: 'replace', path: '/body/name', value: 'b' },
        { op: 'replace', path: '/headers/X-Trace/0', value: '2' },
      ],
      config: { ignored_fields: ['body.other'] },
    })
    const text = wrapper.text()
    expect(text).toContain('1 body')
    expect(text).toContain('1 header')
    expect(text).toContain('1 ignored')
  })
})

describe('DiffViewer patch-op rendering', () => {
  // The patch list badges each row with an op abbreviation (ADD/REM/REP) and
  // sign (+/−/~). PatchOp is typed 'add' | 'remove' | 'replace' — there is no
  // 'move' op to render, so these three cases are the full op surface.
  // The op badge is a font-semibold span holding a nested sign span plus the
  // abbreviation text (e.g. "+ ADD"), so match on class and substring.
  function patchBadge(wrapper: Awaited<ReturnType<typeof mountViewer>>, abbr: string) {
    return wrapper
      .findAll('span')
      .find((s) => s.classes().includes('font-semibold') && s.text().includes(abbr))
  }

  it('renders an add op with the ADD/+ badge and the new value', async () => {
    const wrapper = await mountViewer({
      live: {},
      shadow: { added: 'hello' },
      content: [{ op: 'add', path: '/body/added', value: 'hello' }],
    })
    const badge = patchBadge(wrapper, 'ADD')
    expect(badge).toBeTruthy()
    expect(badge!.text()).toContain('+')
    expect(wrapper.text()).toContain('/body/added')
    expect(wrapper.text()).toContain('hello')
  })

  it('renders a remove op with the REM/− badge and strikes the old value', async () => {
    const wrapper = await mountViewer({
      live: { gone: 'bye' },
      shadow: {},
      content: [{ op: 'remove', path: '/body/gone' }],
    })
    const badge = patchBadge(wrapper, 'REM')
    expect(badge).toBeTruthy()
    expect(badge!.text()).toContain('−')
    const struck = wrapper.findAll('span').find((s) => s.classes().includes('line-through'))
    expect(struck?.text()).toContain('bye')
  })

  it('renders a replace op with the REP/~ badge and old → new values', async () => {
    const wrapper = await mountViewer({
      live: { name: 'a' },
      shadow: { name: 'b' },
      content: [{ op: 'replace', path: '/body/name', value: 'b' }],
    })
    const badge = patchBadge(wrapper, 'REP')
    expect(badge).toBeTruthy()
    expect(badge!.text()).toContain('~')
    expect(wrapper.text()).toContain('→')
    expect(wrapper.text()).toContain('a')
    expect(wrapper.text()).toContain('b')
  })

  it('shows the "No differences" empty state when there are no patch ops', async () => {
    const wrapper = await mountViewer({
      live: { same: 1 },
      shadow: { same: 1 },
      content: [],
    })
    expect(wrapper.text()).toContain('No differences')
    expect(wrapper.text()).toContain('identical')
    // No op badges should be present in the empty state.
    expect(wrapper.findAll('span').some((s) => ['ADD', 'REM', 'REP'].includes(s.text()))).toBe(
      false
    )
  })
})

describe('DiffViewer ignore-field affordance', () => {
  beforeEach(() => {})

  it('emits ignore-field with the pointer converted to a gjson path', async () => {
    const wrapper = await mountViewer({
      live: { user: { name: 'a' } },
      shadow: { user: { name: 'b' } },
      content: [{ op: 'replace', path: '/body/user/name', value: 'b' }],
    })
    const btn = wrapper.find('button[aria-label="Ignore field /body/user/name"]')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    expect(wrapper.emitted('ignore-field')).toBeTruthy()
    expect(wrapper.emitted('ignore-field')![0]).toEqual(['body.user.name'])
  })

  it('hides the ignore button for a field already ignored', async () => {
    const wrapper = await mountViewer({
      live: { user: { name: 'a' } },
      shadow: { user: { name: 'b' } },
      content: [{ op: 'replace', path: '/body/user/name', value: 'b' }],
      config: { ignored_fields: ['body.user.name'] },
    })
    const btn = wrapper.find('button[aria-label="Ignore field /body/user/name"]')
    expect(btn.exists()).toBe(false)
  })
})

describe('DiffViewer body-only comparisons', () => {
  function mountBodyOnly(bodyOnly: boolean, shadowBody: unknown = { id: '1' }) {
    return mount(DiffViewer, {
      props: {
        liveResponse: makeResponse({ id: 1 }, { 'X-Legacy': ['1'] }),
        shadowResponse: makeResponse(shadowBody, { 'X-Graphql': ['1'] }),
        diffContent: [{ op: 'replace', path: '/body/id', value: '1' }],
        diffConfig: makeConfig(),
        bodyOnly,
      },
      global,
    })
  }

  it('leaves headers out of the view when only bodies were compared', async () => {
    const wrapper = mountBodyOnly(true)
    await flushPromises()

    expect(wrapper.text()).not.toContain('"headers"')
    expect(wrapper.text()).not.toContain('header')
    expect(wrapper.text()).toContain('"id"')
  })

  it('still shows headers for regular comparisons', async () => {
    const wrapper = mountBodyOnly(false)
    await flushPromises()

    expect(wrapper.text()).toContain('"headers"')
  })

  it('treats a JSON null body as JSON, not as text', async () => {
    const wrapper = mount(DiffViewer, {
      props: {
        liveResponse: makeResponse({ id: 1 }),
        shadowResponse: makeResponse(null),
        diffContent: [{ op: 'replace', path: '/body', value: null }],
        diffConfig: makeConfig(),
        bodyOnly: true,
      },
      global,
    })
    await flushPromises()

    expect(wrapper.text()).toContain('json')
    expect(wrapper.text()).toContain('null')
  })
})

describe('DiffViewer declared conversions', () => {
  const conversions = [
    { field: 'id', as: 'number' as const, original: '1042' },
    {
      field: 'check_in',
      as: 'date' as const,
      original: '2026-10-09T15:30:00Z',
      error: '"2026-10-09T15:30:00Z" is not at midnight',
    },
  ]

  async function mountWithConversions(view: 'Unified' | 'Split' | 'Patch') {
    const wrapper = mount(DiffViewer, {
      props: {
        // id was converted ("1042" -> 1042) and matches; check_in failed.
        liveResponse: makeResponse({ id: 1042, check_in: '2026-10-09' }),
        shadowResponse: makeResponse({ id: 1042, check_in: '2026-10-09T15:30:00Z' }),
        diffContent: [{ op: 'replace', path: '/body/check_in', value: '2026-10-09T15:30:00Z' }],
        diffConfig: makeConfig(),
        bodyOnly: true,
        conversions,
      },
      global,
    })
    await flushPromises()
    const toggle = wrapper.findAll('button').find((b) => b.text() === view)
    await toggle!.trigger('click')
    await flushPromises()
    return wrapper
  }

  it.each(['Unified', 'Split', 'Patch'] as const)(
    'shows the original value and failures as visible text in the %s view',
    async (view) => {
      const wrapper = await mountWithConversions(view)
      const notes = wrapper.findAll('[data-testid="conversion-note"]').map((n) => n.text())

      if (view === 'Patch') {
        // Only differences are listed: the failed conversion.
        expect(notes).toEqual(['as date failed: "2026-10-09T15:30:00Z" is not at midnight'])
        return
      }
      // Each converted field is annotated once, on the shadow side.
      expect(notes).toEqual([
        'as number · sent "1042"',
        'as date failed: "2026-10-09T15:30:00Z" is not at midnight',
      ])
    }
  )

  it('marks a failed conversion as a danger note', async () => {
    const wrapper = await mountWithConversions('Split')
    const failed = wrapper
      .findAll('[data-testid="conversion-note"]')
      .find((n) => n.text().includes('failed'))
    expect(failed!.classes()).toContain('text-danger')
  })

  it('shows no notes without conversions', async () => {
    const wrapper = mount(DiffViewer, {
      props: {
        liveResponse: makeResponse({ id: 1 }),
        shadowResponse: makeResponse({ id: 1 }),
        diffContent: [],
        diffConfig: makeConfig(),
        bodyOnly: true,
      },
      global,
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="conversion-note"]').exists()).toBe(false)
  })
})
