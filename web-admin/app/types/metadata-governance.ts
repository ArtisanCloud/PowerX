export type MetadataStatus = 'enabled' | 'disabled' | 'archived'

export type MetadataI18nMap = Record<string, string>

export interface MetadataDisplay {
  display_name: string
  display_description?: string
  display_locale: string
  display_locale_missing: boolean
}

export interface MetadataPagination {
  page: number
  page_size: number
  total: number
}

export interface MetadataListPayload<T> {
  items: T[]
  pagination?: MetadataPagination
}

export interface DictionaryNamespace extends MetadataDisplay {
  uuid: string
  namespace: string
  module: string
  name_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  status: MetadataStatus
  item_count: number
}

export interface DictionaryItem extends MetadataDisplay {
  uuid: string
  namespace_uuid: string
  code: string
  label_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  sort_order: number
  status: MetadataStatus
  metadata?: Record<string, unknown>
  reference_count: number
}

export interface Taxonomy extends MetadataDisplay {
  uuid: string
  namespace: string
  module: string
  name_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  max_depth: number
  status: MetadataStatus
}

export interface TaxonomyNode extends MetadataDisplay {
  uuid: string
  taxonomy_uuid: string
  parent_uuid?: string | null
  code: string
  label_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  path: string
  depth: number
  sort_order: number
  status: MetadataStatus
  reference_count: number
  version: number
}

export interface MetadataTag extends MetadataDisplay {
  uuid: string
  namespace: string
  resource_type: string
  code: string
  label_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  color?: string
  status: MetadataStatus
  usage_count: number
}

export interface ResourceType extends MetadataDisplay {
  uuid: string
  resource_type: string
  module: string
  name_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  validator_key?: string
  binding_enabled: boolean
  validator_status: 'available' | 'missing' | 'disabled' | string
  status: MetadataStatus
}

export interface MetadataListParams {
  page?: number
  page_size?: number
  module?: string
  namespace?: string
  resource_type?: string
  status?: string
  q?: string
  locale?: string
}

export interface CreateDictionaryNamespacePayload {
  namespace: string
  module: string
  name_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
}

export interface CreateDictionaryItemPayload {
  code: string
  label_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  sort_order?: number
}

export interface CreateTaxonomyPayload {
  namespace: string
  module: string
  name_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  max_depth: number
}

export interface CreateTaxonomyNodePayload {
  parent_uuid?: string | null
  code: string
  label_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  sort_order?: number
}

export interface CreateMetadataTagPayload {
  namespace: string
  resource_type: string
  code: string
  label_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  color?: string
}

export interface CreateResourceTypePayload {
  resource_type: string
  module: string
  name_i18n: MetadataI18nMap
  description_i18n?: MetadataI18nMap
  validator_key?: string
  binding_enabled: boolean
}

export type MetadataCreateTarget =
  | 'dictionaryNamespace'
  | 'dictionaryItem'
  | 'taxonomy'
  | 'taxonomyNode'
  | 'tag'
  | 'resourceType'

export type MetadataTabKey = 'dictionaries' | 'taxonomies' | 'tags' | 'resourceTypes'

export type MetadataViewState =
  | 'loading'
  | 'no_permission'
  | 'missing_selection'
  | 'empty'
  | 'error'
  | 'ready'
