import { useApiClient } from "../index";
import type {
  CreateDictionaryItemPayload,
  CreateDictionaryNamespacePayload,
  CreateMetadataTagPayload,
  CreateResourceTypePayload,
  CreateTaxonomyNodePayload,
  CreateTaxonomyPayload,
  DictionaryItem,
  DictionaryNamespace,
  MetadataListParams,
  MetadataListPayload,
  MetadataTag,
  ResourceType,
  Taxonomy,
  TaxonomyNode,
} from "~/types/metadata-governance";

type ApiEnvelope<T> = {
  data?: {
    payload?: T
  }
}

const baseUrl = "/admin/metadata";

const payloadOf = <T>(response: unknown): T => {
  const envelope = response as ApiEnvelope<T>;
  return (envelope?.data?.payload ?? response) as T;
};

export const useMetadataGovernanceService = () => {
  const api = useApiClient();

  return {
    listDictionaries(params: MetadataListParams = {}) {
      return api
        .get<ApiEnvelope<MetadataListPayload<DictionaryNamespace>>>(`${baseUrl}/dictionaries`, { params })
        .then(payloadOf<MetadataListPayload<DictionaryNamespace>>);
    },
    createDictionaryNamespace(payload: CreateDictionaryNamespacePayload) {
      return api
        .post<ApiEnvelope<DictionaryNamespace>>(`${baseUrl}/dictionaries`, payload)
        .then(payloadOf<DictionaryNamespace>);
    },
    listDictionaryItems(namespaceUuid: string, params: MetadataListParams = {}) {
      return api
        .get<ApiEnvelope<MetadataListPayload<DictionaryItem>>>(`${baseUrl}/dictionaries/${namespaceUuid}/items`, { params })
        .then(payloadOf<MetadataListPayload<DictionaryItem>>);
    },
    createDictionaryItem(namespaceUuid: string, payload: CreateDictionaryItemPayload) {
      return api
        .post<ApiEnvelope<DictionaryItem>>(`${baseUrl}/dictionaries/${namespaceUuid}/items`, payload)
        .then(payloadOf<DictionaryItem>);
    },
    listTaxonomies(params: MetadataListParams = {}) {
      return api
        .get<ApiEnvelope<MetadataListPayload<Taxonomy>>>(`${baseUrl}/taxonomies`, { params })
        .then(payloadOf<MetadataListPayload<Taxonomy>>);
    },
    createTaxonomy(payload: CreateTaxonomyPayload) {
      return api
        .post<ApiEnvelope<Taxonomy>>(`${baseUrl}/taxonomies`, payload)
        .then(payloadOf<Taxonomy>);
    },
    listTaxonomyNodes(taxonomyUuid: string, params: MetadataListParams = {}) {
      return api
        .get<ApiEnvelope<MetadataListPayload<TaxonomyNode>>>(`${baseUrl}/taxonomies/${taxonomyUuid}/nodes`, { params })
        .then(payloadOf<MetadataListPayload<TaxonomyNode>>);
    },
    createTaxonomyNode(taxonomyUuid: string, payload: CreateTaxonomyNodePayload) {
      return api
        .post<ApiEnvelope<TaxonomyNode>>(`${baseUrl}/taxonomies/${taxonomyUuid}/nodes`, payload)
        .then(payloadOf<TaxonomyNode>);
    },
    listTags(params: MetadataListParams = {}) {
      return api
        .get<ApiEnvelope<MetadataListPayload<MetadataTag>>>(`${baseUrl}/tags`, { params })
        .then(payloadOf<MetadataListPayload<MetadataTag>>);
    },
    createTag(payload: CreateMetadataTagPayload) {
      return api
        .post<ApiEnvelope<MetadataTag>>(`${baseUrl}/tags`, payload)
        .then(payloadOf<MetadataTag>);
    },
    listResourceTypes(params: MetadataListParams = {}) {
      return api
        .get<ApiEnvelope<MetadataListPayload<ResourceType>>>(`${baseUrl}/resource-types`, { params })
        .then(payloadOf<MetadataListPayload<ResourceType>>);
    },
    createResourceType(payload: CreateResourceTypePayload) {
      return api
        .post<ApiEnvelope<ResourceType>>(`${baseUrl}/resource-types`, payload)
        .then(payloadOf<ResourceType>);
    },
  };
};
