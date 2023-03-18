package cmd

import (
	"github.com/hashicorp/terraform/configs"
	"path/filepath"
	"strings"
)

var localModuleSourcePrefixes = []string{
	"./",
	"../",
	".\\",
	"..\\",
}

func joinPath(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

func parseTerraformLocalModuleSource(module *configs.Module) ([]string, error) {
	var sourceMap = map[string]bool{}
	for _, mc := range module.ModuleCalls {
		if isLocalTerraformModuleSource(mc.SourceAddr) && !isExcludedSubModule(mc.SourceAddr) {
			modulePath := joinPath(module.SourceDir, mc.SourceAddr)
			modulePathGlob := joinPath(modulePath, "*.tf*")

			if _, exists := sourceMap[modulePathGlob]; exists {
				continue
			}
			sourceMap[modulePathGlob] = true

			// find local module source recursively
			//subModule, diags := configs.NewParser(nil).LoadConfigDir(modulePath)
			//if diags.HasErrors() {
			//	return nil, errors.New(diags.Error())
			//}
			//subSources, err := parseTerraformLocalModuleSource(subModule)
			//if err != nil {
			//	return nil, err
			//}
			//
			//for _, subSource := range subSources {
			//	sourceMap[subSource] = true
			//}
		}
	}

	var sources = []string{}
	for source := range sourceMap {
		sources = append(sources, source)
	}

	return sources, nil
}

func isExcludedSubModule(addr string) bool {
	for _, module := range localSubModulesExclude {
		if strings.Contains(addr, module) {
			return true
		}
	}

	return false
}

func isLocalTerraformModuleSource(raw string) bool {
	for _, prefix := range localModuleSourcePrefixes {
		if strings.HasPrefix(raw, prefix) {
			return true
		}
	}

	return false
}
