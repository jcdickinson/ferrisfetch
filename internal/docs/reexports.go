package docs

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Reexport represents a pub use that re-exports an item under a different path.
type Reexport struct {
	LocalPrefix  string // Path as seen from the re-exporting crate
	SourceCrate  string // Crate that defines the item
	SourcePrefix string // Path in the source crate
}

// CollectReexports walks the crate's module tree and returns all re-export mappings.
func CollectReexports(crate *RustdocCrate, crateName string) []Reexport {
	var reexports []Reexport
	walkModuleReexports(crate.Root, crate, crateName, &reexports)
	return reexports
}

func walkModuleReexports(moduleID int, crate *RustdocCrate, crateName string, reexports *[]Reexport) {
	moduleItem, ok := crate.Index[strconv.Itoa(moduleID)]
	if !ok {
		return
	}

	modulePath := crateName
	if summary, ok := crate.Paths[strconv.Itoa(moduleID)]; ok {
		modulePath = strings.Join(summary.Path, "::")
	}

	modData := unwrapInner(moduleItem.Inner, "module")
	if modData == nil {
		return
	}

	var mod struct {
		Items []int `json:"items"`
	}
	if err := json.Unmarshal(modData, &mod); err != nil {
		return
	}

	for _, childID := range mod.Items {
		childItem, ok := crate.Index[strconv.Itoa(childID)]
		if !ok {
			continue
		}

		kind := innerKind(childItem.Inner)

		if kind == "module" {
			walkModuleReexports(childID, crate, crateName, reexports)
			continue
		}

		if kind != "use" {
			continue
		}

		useData := unwrapInner(childItem.Inner, "use")
		if useData == nil {
			continue
		}

		var use struct {
			Name   string `json:"name"`
			ID     *int   `json:"id"`
			IsGlob bool   `json:"is_glob"`
		}
		if err := json.Unmarshal(useData, &use); err != nil || use.ID == nil {
			continue
		}

		targetSummary, ok := crate.Paths[strconv.Itoa(*use.ID)]
		if !ok {
			continue
		}

		sourcePath := strings.Join(targetSummary.Path, "::")
		var sourceCrate string

		if targetSummary.CrateID == 0 {
			sourceCrate = crateName
		} else {
			sourceCrate = crate.ExternalCrateName(targetSummary.CrateID)
			if sourceCrate == "" {
				continue
			}
		}

		if use.IsGlob {
			if modulePath == sourcePath && sourceCrate == crateName {
				continue // glob from self — nothing to remap
			}
			*reexports = append(*reexports, Reexport{
				LocalPrefix:  modulePath,
				SourceCrate:  sourceCrate,
				SourcePrefix: sourcePath,
			})
		} else {
			localPath := modulePath + "::" + use.Name
			if localPath == sourcePath && sourceCrate == crateName {
				continue // not a real re-export
			}
			*reexports = append(*reexports, Reexport{
				LocalPrefix:  localPath,
				SourceCrate:  sourceCrate,
				SourcePrefix: sourcePath,
			})
		}
	}
}
