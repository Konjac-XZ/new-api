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
import { Bot, Braces, File, Image, MessageCircle, Wrench } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Markdown } from '@/components/ui/markdown'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

import {
  parseConversationBody,
  type ConversationMessage,
  type ConversationPart,
  type ConversationRole,
} from './conversation-body'

type MessageBodyViewerProps = {
  body: string
  prettyBody: string
}

const roleStyles: Record<ConversationRole, string> = {
  assistant: 'bg-muted/60 border-border/60',
  developer: 'bg-accent/55 border-border/60',
  system: 'bg-muted/35 border-border/60',
  tool: 'bg-secondary/65 border-border/60',
  unknown: 'bg-muted/45 border-border/60',
  user: 'bg-primary text-primary-foreground border-primary',
}

function MessagePartView(props: { part: ConversationPart; user: boolean }) {
  const { t } = useTranslation()

  if (props.part.kind === 'text' || props.part.kind === 'thinking') {
    return (
      <div>
        {props.part.kind === 'thinking' ? (
          <div className='mb-1 text-xs font-medium opacity-70'>
            {t('Thinking')}
          </div>
        ) : null}
        <Markdown
          breaks
          renderHtmlAsText
          className={cn(
            'text-sm [&_p]:my-1 [&_pre]:max-w-full',
            props.user &&
              'text-primary-foreground [&_a]:text-primary-foreground [&_blockquote]:border-primary-foreground/60 [&_code]:bg-primary-foreground/15 [&_pre]:bg-primary-foreground/10'
          )}
        >
          {props.part.text}
        </Markdown>
      </div>
    )
  }

  if (props.part.kind === 'image') {
    const MediaIcon = props.part.label === 'Document' ? File : Image
    return (
      <div className='flex min-w-0 items-center gap-2 text-sm'>
        <MediaIcon className='size-4 shrink-0' aria-hidden='true' />
        <span className='shrink-0 font-medium'>
          {t(props.part.label || 'Image')}
        </span>
        {props.part.text ? (
          <span className='min-w-0 truncate font-mono text-xs opacity-75'>
            {props.part.text}
          </span>
        ) : null}
      </div>
    )
  }

  return (
    <div className='min-w-0'>
      <div className='mb-1 flex items-center gap-1.5 text-xs font-medium opacity-70'>
        <Wrench className='size-3.5' aria-hidden='true' />
        {props.part.label || t('Tool')}
      </div>
      <pre className='bg-foreground/10 max-w-full overflow-x-auto rounded-md p-2 font-mono text-xs [overflow-wrap:anywhere] whitespace-pre-wrap'>
        {props.part.text}
      </pre>
    </div>
  )
}

function getRoleLabel(
  role: ConversationRole,
  originalRole: string,
  translate: (key: string) => string
): string {
  if (role === 'unknown') return originalRole
  const labels: Record<Exclude<ConversationRole, 'unknown'>, string> = {
    assistant: 'Assistant',
    developer: 'Developer',
    system: 'System',
    tool: 'Tool',
    user: 'User',
  }
  return translate(labels[role])
}

function RoleIcon(props: { role: ConversationRole }) {
  if (props.role === 'assistant') {
    return <Bot className='size-3.5' aria-hidden='true' />
  }
  if (props.role === 'tool') {
    return <Wrench className='size-3.5' aria-hidden='true' />
  }
  return <MessageCircle className='size-3.5' aria-hidden='true' />
}

function getOccurrenceKey(
  value: ConversationMessage | ConversationPart,
  occurrences: Map<string, number>
): string {
  const serialized = JSON.stringify(value)
  const occurrence = occurrences.get(serialized) ?? 0
  occurrences.set(serialized, occurrence + 1)
  return `${serialized}-${occurrence}`
}

function MessageBubble(props: { message: ConversationMessage; index: number }) {
  const { t } = useTranslation()
  const isUser = props.message.role === 'user'
  const isSystem =
    props.message.role === 'system' || props.message.role === 'developer'
  const label = getRoleLabel(props.message.role, props.message.originalRole, t)
  const partOccurrences = new Map<string, number>()

  return (
    <article
      aria-label={`${label} ${t('Message')} ${props.index + 1}`}
      className={cn(
        'flex w-full flex-col',
        isUser ? 'items-end' : 'items-start',
        isSystem && 'items-center'
      )}
    >
      <div
        className={cn(
          'mb-1 flex items-center gap-1.5 px-1 text-xs font-medium',
          'text-muted-foreground',
          isUser && 'text-primary'
        )}
      >
        <RoleIcon role={props.message.role} />
        <span>
          {props.message.name ? `${label} · ${props.message.name}` : label}
        </span>
      </div>
      <div
        className={cn(
          'flex max-w-[92%] flex-col gap-3 rounded-2xl border px-3.5 py-2.5 shadow-xs sm:max-w-[82%]',
          isUser ? 'rounded-br-sm' : 'rounded-bl-sm',
          isSystem && 'w-full max-w-full rounded-lg',
          roleStyles[props.message.role]
        )}
      >
        {props.message.parts.length > 0 ? (
          props.message.parts.map((part) => (
            <MessagePartView
              key={getOccurrenceKey(part, partOccurrences)}
              part={part}
              user={isUser}
            />
          ))
        ) : (
          <span className='text-sm opacity-60'>{t('Empty content')}</span>
        )}
      </div>
    </article>
  )
}

export function MessageBodyViewer(props: MessageBodyViewerProps) {
  const { t } = useTranslation()
  const conversation = useMemo(
    () => parseConversationBody(props.body),
    [props.body]
  )

  if (!conversation) {
    return (
      <Textarea
        readOnly
        value={props.prettyBody}
        className='h-auto min-h-0 resize-none font-mono text-xs leading-relaxed'
      />
    )
  }

  const messageOccurrences = new Map<string, number>()

  return (
    <Tabs defaultValue='conversation'>
      <TabsList className='ml-auto grid w-full grid-cols-2 sm:absolute sm:top-0 sm:right-0 sm:w-72'>
        <TabsTrigger value='conversation'>
          <MessageCircle aria-hidden='true' />
          {t('Conversation')}
        </TabsTrigger>
        <TabsTrigger value='raw'>
          <Braces aria-hidden='true' />
          {t('Raw JSON')}
        </TabsTrigger>
      </TabsList>
      <TabsContent value='conversation' className='mt-3'>
        <div className='bg-background flex max-h-[36rem] flex-col gap-4 overflow-y-auto rounded-lg border p-3 sm:p-4'>
          {conversation.messages.map((message, index) => (
            <MessageBubble
              key={getOccurrenceKey(message, messageOccurrences)}
              message={message}
              index={index}
            />
          ))}
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
