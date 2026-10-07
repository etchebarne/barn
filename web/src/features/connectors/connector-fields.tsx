import { SecretInput } from "@/components/secret-input"
import { Badge } from "@/components/ui/badge"
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

import { fieldId, type FormField, type FormValues } from "./logic"

/** Inputs generated from a connector type's credential and config fields. */
export function ConnectorFields({
  fields,
  values,
  errors,
  disabled,
  onChange,
  idPrefix,
}: {
  fields: FormField[]
  values: FormValues
  errors: Record<string, string>
  disabled?: boolean
  onChange: (id: string, value: string) => void
  idPrefix: string
}) {
  return (
    <>
      {fields.map((field) => {
        const id = fieldId(field)
        const inputId = `${idPrefix}-${id}`
        const error = errors[id]
        const props = {
          id: inputId,
          value: values[id] ?? "",
          disabled,
          autoComplete: "off",
          "aria-invalid": error ? true : undefined,
          placeholder: field.saved ? "Saved. Leave empty to keep it" : undefined,
          onChange: (e: { target: { value: string } }) => onChange(id, e.target.value),
        }
        return (
          <Field key={id} data-invalid={!!error || undefined}>
            <FieldLabel htmlFor={inputId} className="gap-2">
              {field.label}
              {field.optional && (
                <span className="text-xs font-normal text-muted-foreground">(optional)</span>
              )}
              {field.saved && (
                <Badge variant="secondary" className="font-normal">
                  Saved
                </Badge>
              )}
            </FieldLabel>
            {field.input === "secret" ? (
              <SecretInput {...props} />
            ) : (
              <Input type="text" {...props} />
            )}
            {error ? (
              <FieldError>{error}</FieldError>
            ) : (
              field.help && <FieldDescription>{field.help}</FieldDescription>
            )}
          </Field>
        )
      })}
    </>
  )
}
