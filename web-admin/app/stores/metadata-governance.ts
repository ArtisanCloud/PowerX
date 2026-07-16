import { defineStore } from "pinia";
import { useMetadataGovernanceService } from "~/composables/api/services/metadataGovernanceService";
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
  MetadataPagination,
  MetadataTag,
  MetadataTabKey,
  MetadataViewState,
  ResourceType,
  Taxonomy,
  TaxonomyNode,
} from "~/types/metadata-governance";

interface MetadataGovernanceState {
  activeTab: MetadataTabKey
  loading: boolean
  error: string | null
  hasReadPermission: boolean
  hasManagePermission: boolean
  dictionaries: DictionaryNamespace[]
  dictionaryItems: DictionaryItem[]
  selectedDictionaryUuid: string
  taxonomies: Taxonomy[]
  taxonomyNodes: TaxonomyNode[]
  selectedTaxonomyUuid: string
  tags: MetadataTag[]
  resourceTypes: ResourceType[]
  pagination: Partial<Record<MetadataTabKey | "dictionaryItems" | "taxonomyNodes", MetadataPagination>>
}

const defaultPage = { page: 1, page_size: 20 };

const errorMessage = (err: unknown) =>
  err && typeof err === "object" && "message" in err ? String((err as any).message) : "metadata_governance.error";

export const useMetadataGovernanceStore = defineStore("metadata-governance", {
  state: (): MetadataGovernanceState => ({
    activeTab: "dictionaries",
    loading: false,
    error: null,
    hasReadPermission: true,
    hasManagePermission: false,
    dictionaries: [],
    dictionaryItems: [],
    selectedDictionaryUuid: "",
    taxonomies: [],
    taxonomyNodes: [],
    selectedTaxonomyUuid: "",
    tags: [],
    resourceTypes: [],
    pagination: {},
  }),
  getters: {
    tabState: (state) => (tab: MetadataTabKey): MetadataViewState => {
      if (!state.hasReadPermission) return "no_permission";
      if (state.loading) return "loading";
      if (state.error) return "error";
      if (tab === "dictionaries" && state.dictionaries.length > 0 && !state.selectedDictionaryUuid) return "missing_selection";
      if (tab === "taxonomies" && state.taxonomies.length > 0 && !state.selectedTaxonomyUuid) return "missing_selection";
      if (tab === "dictionaries" && state.dictionaries.length === 0) return "empty";
      if (tab === "taxonomies" && state.taxonomies.length === 0) return "empty";
      if (tab === "tags" && state.tags.length === 0) return "empty";
      if (tab === "resourceTypes" && state.resourceTypes.length === 0) return "empty";
      return "ready";
    },
  },
  actions: {
    setPermissions(read: boolean, manage: boolean) {
      this.hasReadPermission = read;
      this.hasManagePermission = manage;
    },
    async fetchDictionaries(params: MetadataListParams = {}, detailParams: MetadataListParams = params) {
      if (!this.hasReadPermission) return;
      this.loading = true;
      this.error = null;
      try {
        const service = useMetadataGovernanceService();
        const payload = await service.listDictionaries({ ...defaultPage, ...params });
        this.dictionaries = payload.items ?? [];
        this.pagination.dictionaries = payload.pagination;
        const selectedExists = this.dictionaries.some((item) => item.uuid === this.selectedDictionaryUuid);
        if ((!this.selectedDictionaryUuid || !selectedExists) && this.dictionaries.length > 0) {
          this.selectedDictionaryUuid = this.dictionaries[0].uuid;
        }
        if (this.dictionaries.length === 0) {
          this.selectedDictionaryUuid = "";
          this.dictionaryItems = [];
        }
        if (this.selectedDictionaryUuid) {
          await this.fetchDictionaryItems(this.selectedDictionaryUuid, detailParams);
        }
      } catch (err) {
        this.error = errorMessage(err);
      } finally {
        this.loading = false;
      }
    },
    async fetchDictionaryItems(namespaceUuid: string, params: MetadataListParams = {}) {
      if (!this.hasReadPermission || !namespaceUuid) return;
      const service = useMetadataGovernanceService();
      const payload = await service.listDictionaryItems(namespaceUuid, { ...defaultPage, ...params });
      this.dictionaryItems = payload.items ?? [];
      this.pagination.dictionaryItems = payload.pagination;
    },
    async selectDictionary(namespaceUuid: string, params: MetadataListParams = {}) {
      this.selectedDictionaryUuid = namespaceUuid;
      this.loading = true;
      this.error = null;
      try {
        await this.fetchDictionaryItems(namespaceUuid, params);
      } catch (err) {
        this.error = errorMessage(err);
      } finally {
        this.loading = false;
      }
    },
    async fetchTaxonomies(params: MetadataListParams = {}, detailParams: MetadataListParams = params) {
      if (!this.hasReadPermission) return;
      this.loading = true;
      this.error = null;
      try {
        const service = useMetadataGovernanceService();
        const payload = await service.listTaxonomies({ ...defaultPage, ...params });
        this.taxonomies = payload.items ?? [];
        this.pagination.taxonomies = payload.pagination;
        const selectedExists = this.taxonomies.some((item) => item.uuid === this.selectedTaxonomyUuid);
        if ((!this.selectedTaxonomyUuid || !selectedExists) && this.taxonomies.length > 0) {
          this.selectedTaxonomyUuid = this.taxonomies[0].uuid;
        }
        if (this.taxonomies.length === 0) {
          this.selectedTaxonomyUuid = "";
          this.taxonomyNodes = [];
        }
        if (this.selectedTaxonomyUuid) {
          await this.fetchTaxonomyNodes(this.selectedTaxonomyUuid, detailParams);
        }
      } catch (err) {
        this.error = errorMessage(err);
      } finally {
        this.loading = false;
      }
    },
    async fetchTaxonomyNodes(taxonomyUuid: string, params: MetadataListParams = {}) {
      if (!this.hasReadPermission || !taxonomyUuid) return;
      const service = useMetadataGovernanceService();
      const payload = await service.listTaxonomyNodes(taxonomyUuid, params);
      this.taxonomyNodes = payload.items ?? [];
      this.pagination.taxonomyNodes = payload.pagination;
    },
    async selectTaxonomy(taxonomyUuid: string, params: MetadataListParams = {}) {
      this.selectedTaxonomyUuid = taxonomyUuid;
      this.loading = true;
      this.error = null;
      try {
        await this.fetchTaxonomyNodes(taxonomyUuid, params);
      } catch (err) {
        this.error = errorMessage(err);
      } finally {
        this.loading = false;
      }
    },
    async fetchTags(params: MetadataListParams = {}) {
      if (!this.hasReadPermission) return;
      this.loading = true;
      this.error = null;
      try {
        const service = useMetadataGovernanceService();
        const payload = await service.listTags({ ...defaultPage, ...params });
        this.tags = payload.items ?? [];
        this.pagination.tags = payload.pagination;
      } catch (err) {
        this.error = errorMessage(err);
      } finally {
        this.loading = false;
      }
    },
    async fetchResourceTypes(params: MetadataListParams = {}) {
      if (!this.hasReadPermission) return;
      this.loading = true;
      this.error = null;
      try {
        const service = useMetadataGovernanceService();
        const payload = await service.listResourceTypes({ ...defaultPage, ...params });
        this.resourceTypes = payload.items ?? [];
        this.pagination.resourceTypes = payload.pagination;
      } catch (err) {
        this.error = errorMessage(err);
      } finally {
        this.loading = false;
      }
    },
    async createDictionaryNamespace(payload: CreateDictionaryNamespacePayload) {
      const service = useMetadataGovernanceService();
      return await service.createDictionaryNamespace(payload);
    },
    async createDictionaryItem(namespaceUuid: string, payload: CreateDictionaryItemPayload) {
      const service = useMetadataGovernanceService();
      return await service.createDictionaryItem(namespaceUuid, payload);
    },
    async createTaxonomy(payload: CreateTaxonomyPayload) {
      const service = useMetadataGovernanceService();
      return await service.createTaxonomy(payload);
    },
    async createTaxonomyNode(taxonomyUuid: string, payload: CreateTaxonomyNodePayload) {
      const service = useMetadataGovernanceService();
      return await service.createTaxonomyNode(taxonomyUuid, payload);
    },
    async createTag(payload: CreateMetadataTagPayload) {
      const service = useMetadataGovernanceService();
      return await service.createTag(payload);
    },
    async createResourceType(payload: CreateResourceTypePayload) {
      const service = useMetadataGovernanceService();
      return await service.createResourceType(payload);
    },
  },
});
