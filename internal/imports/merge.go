package imports

import "strings"

// MergeWithSkeleton overlays enriched data onto a skeleton file.
//
// The skeleton defines the authoritative resource set and preserves any user-provided values.
// Enriched fields only fill in missing values or replace placeholder IDs.
func MergeWithSkeleton(skeleton, enriched *File) *File {
	switch {
	case skeleton == nil:
		return enriched
	case enriched == nil:
		return skeleton
	}

	result := &File{
		NameTable: mergeNameTables(skeleton.NameTable, enriched.NameTable),
		Resources: mergeResources(skeleton.Resources, enriched.Resources),
	}
	return result
}

func mergeNameTables(skeleton, enriched map[string]string) map[string]string {
	if len(skeleton) == 0 && len(enriched) == 0 {
		return nil
	}
	out := make(map[string]string, len(skeleton)+len(enriched))
	for k, v := range enriched {
		if v == "" {
			continue
		}
		out[k] = v
	}
	for k, v := range skeleton {
		if v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func mergeResources(skeleton, enriched []Resource) []Resource {
	index := make(map[string]Resource, len(enriched))
	for _, res := range enriched {
		if key := mergeKey(res); key != "" {
			index[key] = res
		}
	}

	merged := make([]Resource, 0, len(skeleton)+len(enriched))
	for _, res := range skeleton {
		key := mergeKey(res)
		if candidate, ok := index[key]; ok {
			merged = append(merged, mergeResource(res, candidate))
			delete(index, key)
			continue
		}
		merged = append(merged, res)
	}

	for _, res := range index {
		merged = append(merged, res)
	}

	sortResources(merged)
	return merged
}

func mergeResource(skeleton, enriched Resource) Resource {
	result := skeleton

	if result.Type == "" && enriched.Type != "" {
		result.Type = enriched.Type
	}
	if result.Name == "" && enriched.Name != "" {
		result.Name = enriched.Name
	}
	if result.LogicalName == "" && enriched.LogicalName != "" {
		result.LogicalName = enriched.LogicalName
	}
	result.ID = chooseID(result.ID, enriched.ID)
	if len(result.Properties) == 0 && len(enriched.Properties) > 0 {
		result.Properties = cloneStrings(enriched.Properties)
	}
	if !result.Component {
		result.Component = enriched.Component
	}
	if result.Version == "" && enriched.Version != "" {
		result.Version = enriched.Version
	}
	if result.Parent == "" && enriched.Parent != "" {
		result.Parent = enriched.Parent
	}
	if result.Provider == "" && enriched.Provider != "" {
		result.Provider = enriched.Provider
	}

	return result
}

func chooseID(current, candidate string) string {
	switch {
	case candidate == "":
		return current
	case candidate == placeholderID:
		if current == "" {
			return candidate
		}
		if strings.EqualFold(current, placeholderID) {
			return candidate
		}
		return current
	case current == "" || strings.EqualFold(current, placeholderID):
		return candidate
	default:
		// Preserve any explicit/non-placeholder ID from the skeleton.
		return current
	}
}

func mergeKey(res Resource) string {
	if res.Type == "" {
		return ""
	}
	name := res.Name
	if name == "" {
		name = res.LogicalName
	}
	if name == "" {
		return ""
	}
	return res.Type + "|" + name
}
