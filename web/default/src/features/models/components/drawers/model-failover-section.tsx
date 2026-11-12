/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Controller, useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { SideDrawerSection } from '@/components/drawer-layout'
import {
  modelFailoverFormSchema,
  type ModelFailoverFormValues,
} from '../../lib/model-failover'

export function ModelFailoverSection() {
  const { t } = useTranslation()
  const form = useFormContext<ModelFailoverFormValues>()
  const enabled = useWatch({ control: form.control, name: 'failover_enabled' })

  return (
    <SideDrawerSection>
      <FieldSet>
        <FieldLegend variant='label'>{t('Channel Failover')}</FieldLegend>
        <FieldGroup>
          <Controller
            control={form.control}
            name='failover_enabled'
            render={({ field }) => (
              <Field orientation='horizontal'>
                <FieldContent>
                  <FieldLabel htmlFor='model-failover-enabled'>
                    {t('Automatic channel failover')}
                  </FieldLabel>
                  <FieldDescription id='model-failover-description'>
                    {t(
                      'On retryable upstream errors, switch to another channel for the same model and group. Failed channels are not retried within the request. When disabled, global retry settings apply.'
                    )}
                  </FieldDescription>
                </FieldContent>
                <Switch
                  id='model-failover-enabled'
                  name={field.name}
                  ref={field.ref}
                  onBlur={field.onBlur}
                  checked={field.value}
                  onCheckedChange={(checked) => {
                    field.onChange(checked)
                    if (
                      !checked &&
                      !modelFailoverFormSchema.shape.failover_max_retries.safeParse(
                        form.getValues('failover_max_retries')
                      ).success
                    ) {
                      form.setValue('failover_max_retries', 2, {
                        shouldValidate: true,
                      })
                    }
                  }}
                  aria-describedby='model-failover-description'
                />
              </Field>
            )}
          />
          {enabled && (
            <Controller
              control={form.control}
              name='failover_max_retries'
              render={({ field, fieldState }) => (
                <Field data-invalid={fieldState.invalid}>
                  <FieldLabel htmlFor='model-failover-retries'>
                    {t('Maximum channel switches')}
                  </FieldLabel>
                  <Input
                    {...field}
                    value={Number.isNaN(field.value) ? '' : field.value}
                    id='model-failover-retries'
                    type='number'
                    min={1}
                    max={10}
                    step={1}
                    onChange={(event) =>
                      field.onChange(event.target.valueAsNumber)
                    }
                    aria-invalid={fieldState.invalid}
                    aria-describedby='model-failover-retries-description model-failover-retries-error'
                  />
                  <FieldDescription id='model-failover-retries-description'>
                    {t(
                      '1–10 switches after the initial attempt, replacing the global retry limit for this model. Channel priority and weight still apply. No switching after a response has started or for requests pinned to a channel.'
                    )}
                  </FieldDescription>
                  {fieldState.invalid && (
                    <FieldError id='model-failover-retries-error'>
                      {t('Enter an integer from 1 to 10.')}
                    </FieldError>
                  )}
                </Field>
              )}
            />
          )}
        </FieldGroup>
      </FieldSet>
    </SideDrawerSection>
  )
}
