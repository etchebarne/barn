import { MonitorIcon, MoonIcon, SunIcon } from "lucide-react"

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { isThemePreference, useThemeStore, type ThemePreference } from "@/lib/theme"

import { SettingsSection } from "./settings-section"

const OPTIONS: { value: ThemePreference; label: string; icon: typeof SunIcon }[] = [
  { value: "light", label: "Light", icon: SunIcon },
  { value: "dark", label: "Dark", icon: MoonIcon },
  { value: "system", label: "System", icon: MonitorIcon },
]

export function AppearanceSection() {
  const preference = useThemeStore((s) => s.preference)
  const setPreference = useThemeStore((s) => s.setPreference)

  return (
    <SettingsSection id="appearance" title="Appearance" description="Theme for this device.">
      <ToggleGroup
        variant="outline"
        aria-label="Theme"
        value={[preference]}
        onValueChange={(value: unknown[]) => {
          const next = value[0]
          // Ignore deselecting the active option: one theme is always selected.
          if (isThemePreference(next)) setPreference(next)
        }}
      >
        {OPTIONS.map(({ value, label, icon: Icon }) => (
          <ToggleGroupItem key={value} value={value} className="select-none">
            <Icon />
            {label}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </SettingsSection>
  )
}
