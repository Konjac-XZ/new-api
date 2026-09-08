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
import { Brain, Braces, ChevronDown, FileText } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Markdown } from '@/components/ui/markdown'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

import { parseResponseBody, type ResponseBodyPart } from './response-body'

type ResponseBodyViewerProps = {
  body: string
  prettyBody: string
}

function ThinkingBlock(props: { children: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='rounded-lg border'
    >
      <CollapsibleTrigger className='hover:bg-muted/50 flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm font-medium'>
        <Brain className='text-muted-foreground size-4' aria-hidden='true' />
        <span className='flex-1'>{t('Thinking')}</span>
        <ChevronDown
          className={cn(
            'text-muted-foreground size-4 transition-transform',
            open && 'rotate-180'
          )}
          aria-hidden='true'
        />
      </CollapsibleTrigger>
      <CollapsibleContent className='border-t px-3 py-2'>
        <Markdown
          breaks
          renderHtmlAsText
          className='text-muted-foreground text-sm'
        >
          {props.children}
        </Markdown>
      </CollapsibleContent>
    </Collapsible>
  )
}

function getPartKey(part: ResponseBodyPart, occurrence: number): string {
  return `${part.kind}-${part.text}-${occurrence}`
}

export function ResponseBodyViewer(props: ResponseBodyViewerProps) {
  const { t } = useTranslation()
  const parts = useMemo(() => parseResponseBody(props.body), [props.body])

  if (!parts) {
    return (
      <Textarea
        readOnly
        value={props.prettyBody}
        className='h-auto min-h-0 resize-none font-mono text-xs leading-relaxed'
      />
    )
  }

  const occurrences = new Map<string, number>()
  return (
    <Tabs defaultValue='content'>
      <TabsList className='ml-auto grid w-full grid-cols-2 sm:absolute sm:top-0 sm:right-0 sm:w-72'>
        <TabsTrigger value='content'>
          <FileText aria-hidden='true' />
          {t('Content')}
        </TabsTrigger>
        <TabsTrigger value='raw'>
          <Braces aria-hidden='true' />
          {t('Raw JSON')}
        </TabsTrigger>
      </TabsList>
      <TabsContent value='content' className='mt-3'>
        <div className='bg-background flex max-h-[36rem] flex-col gap-3 overflow-y-auto rounded-lg border p-3 sm:p-4'>
          {parts.map((part) => {
            const occurrence = occurrences.get(part.kind) ?? 0
            occurrences.set(part.kind, occurrence + 1)
            if (part.kind === 'thinking') {
              return (
                <ThinkingBlock key={getPartKey(part, occurrence)}>
                  {part.text}
                </ThinkingBlock>
              )
            }
            return (
              <Markdown
                key={getPartKey(part, occurrence)}
                breaks
                renderHtmlAsText
                className='text-sm'
              >
                {part.text}
              </Markdown>
            )
          })}
        </div>
      </TabsContent>
      <TabsContent value='raw' className='mt-3'>
        <Textarea
          readOnly
          value={props.prettyBody}
          className='h-auto min-h-0 resize-none font-mono text-xs leading-relaxed'
        />
      </TabsContent>
    </Tabs>
  )
}
