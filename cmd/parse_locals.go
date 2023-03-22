package cmd

// Terragrunt doesn't give us an easy way to access all of the Locals from a module
// in an easy to digest way. This file is mostly just follows along how Terragrunt
// parses the `locals` blocks and evaluates their contents.

import (
	"github.com/hashicorp/terraform/configs"
	"github.com/zclconf/go-cty/cty"
	"path/filepath"
)

// ResolvedLocals are the parsed result of local values this module cares about
type ResolvedLocals struct {
	// The Atlantis workflow to use for some project
	AtlantisWorkflow string

	// Apply requirements to override the global `--apply-requirements` flag
	ApplyRequirements []string

	// Extra dependencies that can be hardcoded in config
	ExtraAtlantisDependencies []string

	// If set, a single module will have autoplan turned to this setting
	AutoPlan *bool

	// If set to true, the module will not be included in the output
	Skip *bool

	// Terraform version to use just for this project
	TerraformVersion string

	// If set to true, create Atlantis project
	markedProject *bool

	ExecutionOrderGroup int
}

func resolveLocals(module *configs.Module) ResolvedLocals {
	resolved := ResolvedLocals{}
	locals := module.Locals

	atlantisMap, ok := locals["atlantis"]

	if len(locals) == 0 || !ok {
		return resolved
	}

	atlantisValues, diag := atlantisMap.Expr.Value(nil)
	if diag.HasErrors() {
		return resolved
	}
	values := atlantisValues.AsValueMap()

	workflowValue, ok := values["workflow"]
	if ok {
		if workflowValue.Type().IsPrimitiveType() {
			resolved.AtlantisWorkflow = workflowValue.AsString()
		}
	}

	executionOrderGroup, ok := values["execution_order_group"]
	if ok {
		if executionOrderGroup.Type().Equals(cty.Number) {
			intValue, _ := executionOrderGroup.AsBigFloat().Int64()
			resolved.ExecutionOrderGroup = int(intValue)
		}
	}

	versionValue, ok := values["terraform_version"]
	if ok {
		if versionValue.Type().IsPrimitiveType() {
			resolved.TerraformVersion = versionValue.AsString()
		}
	}

	autoPlanValue, ok := values["autoplan"]
	if ok {
		if autoPlanValue.Type().Equals(cty.Bool) {
			hasValue := autoPlanValue.True()
			resolved.AutoPlan = &hasValue
		}
	}

	skipValue, ok := values["skip"]
	if ok {
		if skipValue.Type().Equals(cty.Bool) {
			hasValue := skipValue.True()
			resolved.Skip = &hasValue
		}
	}

	applyReqs, ok := values["apply_requirements"]
	if ok {
		if applyReqs.Type().IsTupleType() {
			it := applyReqs.ElementIterator()
			for it.Next() {
				_, val := it.Element()
				resolved.ApplyRequirements = append(resolved.ApplyRequirements, filepath.ToSlash(val.AsString()))
			}
		}
	}

	extraDependencies, ok := values["extra_dependencies"]
	if ok {
		if extraDependencies.Type().IsTupleType() {
			it := extraDependencies.ElementIterator()
			for it.Next() {
				_, val := it.Element()
				resolved.ExtraAtlantisDependencies = append(resolved.ExtraAtlantisDependencies, filepath.ToSlash(val.AsString()))
			}
		}
	}

	return resolved
}
