package imports

import "strings"

// MergeWithSkeleton overlays an enriched import file onto a skeleton produced by `pulumi preview --import-file`.
// The skeleton defines the authoritative resource set; enriched fields replace placeholders/missing data.
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
	for k, v := range skeleton {
		out[k] = v
	}
	for k, v := range enriched {
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

	if enriched.Type != "" {
		result.Type = enriched.Type
	}
	if enriched.Name != "" {
		result.Name = enriched.Name
	}
	if enriched.LogicalName != "" {
		result.LogicalName = enriched.LogicalName
	}
	result.ID = chooseID(result.ID, enriched.ID)
	if len(enriched.Properties) > 0 {
		result.Properties = cloneStrings(enriched.Properties)
	}
	result.Component = result.Component || enriched.Component
	if enriched.Version != "" {
		result.Version = enriched.Version
	}
	if enriched.Parent != "" {
		result.Parent = enriched.Parent
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
		return candidate
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
