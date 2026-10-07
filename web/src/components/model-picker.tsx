import { useQuery } from "@tanstack/react-query"
import type { Ref } from "react"

import { Button } from "@/components/ui/button"
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { modelsQueryOptions } from "@/lib/models"

/**
 * Searchable model select, filled from `GET /models`. The one model picker used everywhere
 * (onboarding, agent details). Handles loading and load errors itself.
 */
export function ModelPicker({
  id,
  value,
  onValueChange,
  disabled,
  invalid,
  inputRef,
}: {
  id?: string
  inputRef?: Ref<HTMLInputElement>
  value: string | null
  onValueChange: (model: string | null) => void
  disabled?: boolean
  invalid?: boolean
}) {
  const models = useQuery(modelsQueryOptions)
  const ids = models.data?.map((m) => m.id) ?? []
  // Keep the current value selectable even if the provider no longer lists it.
  const items = value && !ids.includes(value) ? [value, ...ids] : ids

  if (models.error) {
    return (
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="text-destructive">Couldn't load models: {models.error.message}</span>
        <Button type="button" variant="outline" size="sm" onClick={() => void models.refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  return (
    <Combobox items={items} value={value} onValueChange={onValueChange}>
      <ComboboxInput
        ref={inputRef}
        id={id}
        className="w-full"
        placeholder={models.isPending ? "Loading models…" : "Search models"}
        disabled={disabled || models.isPending}
        aria-invalid={invalid || undefined}
      />
      <ComboboxContent>
        <ComboboxEmpty>No models match.</ComboboxEmpty>
        <ComboboxList>
          {(item: string) => (
            <ComboboxItem key={item} value={item}>
              {item}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}
