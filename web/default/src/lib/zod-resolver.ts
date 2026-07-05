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
import { zodResolver as baseZodResolver } from '@hookform/resolvers/zod'
import type { FieldValues, Resolver } from 'react-hook-form'

type ZodResolverOptions = {
  mode?: 'async' | 'sync'
  raw?: boolean
}

type ZodResolverFunction = (
  schema: unknown,
  schemaOptions?: unknown,
  resolverOptions?: ZodResolverOptions
) => Resolver<FieldValues, unknown, FieldValues>

const resolveZodSchema = baseZodResolver as ZodResolverFunction

export function zodResolver<
  TFieldValues extends FieldValues = FieldValues,
  TContext = unknown,
  TTransformedValues extends FieldValues = TFieldValues,
>(
  schema: unknown,
  schemaOptions?: unknown,
  resolverOptions?: ZodResolverOptions
): Resolver<TFieldValues, TContext, TTransformedValues> {
  return resolveZodSchema(schema, schemaOptions, resolverOptions) as Resolver<
    TFieldValues,
    TContext,
    TTransformedValues
  >
}
