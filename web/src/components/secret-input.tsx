import { EyeIcon, EyeOffIcon } from "lucide-react"
import { useState, type ComponentProps } from "react"

import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"

/**
 * Password-style input that's hidden by default, with an explicit show/hide toggle.
 * Used for passwords and API keys.
 */
export function SecretInput({
  className,
  ...props
}: Omit<ComponentProps<typeof InputGroupInput>, "type">) {
  const [visible, setVisible] = useState(false)
  return (
    <InputGroup className={className}>
      <InputGroupInput
        type={visible ? "text" : "password"}
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        {...props}
      />
      <InputGroupAddon align="inline-end">
        <InputGroupButton
          size="icon-xs"
          aria-label={visible ? "Hide" : "Show"}
          aria-pressed={visible}
          onClick={() => setVisible((v) => !v)}
        >
          {visible ? <EyeOffIcon /> : <EyeIcon />}
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
  )
}
