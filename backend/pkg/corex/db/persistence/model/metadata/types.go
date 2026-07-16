package metadata

const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
	StatusArchived = "archived"
)

const (
	MetadataTypeDictionaryItem = "dictionary_item"
	MetadataTypeTaxonomyNode   = "taxonomy_node"
)

const (
	SourceAdmin  = "admin"
	SourceSeed   = "seed"
	SourcePlugin = "plugin"
	SourceSystem = "system"
)

const (
	TableDictionaryNamespaces = "metadata_dictionary_namespaces"
	TableDictionaryItems      = "metadata_dictionary_items"
	TableTaxonomies           = "metadata_taxonomies"
	TableTaxonomyNodes        = "metadata_taxonomy_nodes"
	TableTags                 = "metadata_tags"
	TableTagBindings          = "metadata_tag_bindings"
	TableResourceTypes        = "metadata_resource_types"
	TableReferences           = "metadata_references"
)

func IsValidStatus(status string) bool {
	switch status {
	case StatusEnabled, StatusDisabled, StatusArchived:
		return true
	default:
		return false
	}
}
