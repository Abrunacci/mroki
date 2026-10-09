<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRightLeft } from 'lucide-vue-next'
import { shadowAdapterLabel } from '@/lib/shadow-adapter'

interface Props {
  /** Adapter type, e.g. "graphql". */
  type: string
  /** Mapping version; shown next to the label when given. */
  version?: string
}

const props = defineProps<Props>()

const label = computed(() => shadowAdapterLabel(props.type))
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 text-xs px-2 py-0.5 rounded-full border border-info/30 bg-info/10 text-info whitespace-nowrap shrink-0"
    :title="version ? `Mapping version ${version}` : undefined"
  >
    <ArrowRightLeft class="h-3 w-3" aria-hidden="true" />
    {{ label }}
    <code v-if="version" class="font-mono text-[10px] text-muted-foreground">{{ version }}</code>
  </span>
</template>
