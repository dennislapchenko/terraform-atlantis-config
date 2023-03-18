package cmd

// Terragrunt doesn't give us an easy way to access all of the Locals from a module
// in an easy to digest way. This file is mostly just follows along how Terragrunt
// parses the `locals` blocks and evaluates their contents.

import (
	"github.com/hashicorp/terraform/configs"
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
}

func resolveLocals(module *configs.Module) ResolvedLocals {
	resolved := ResolvedLocals{}
	locals := module.Locals

	// Return an empty set of locals if no `locals` block was present
	if len(locals) == 0 {
		return resolved
	}

	workflowValue, ok := locals["atlantis_workflow"]
	if ok {
		val, diag := workflowValue.Expr.Value(nil)
		if !diag.HasErrors() {
			resolved.AtlantisWorkflow = val.AsString()
		}
	}

	versionValue, ok := locals["atlantis_terraform_version"]
	if ok {
		val, diag := versionValue.Expr.Value(nil)
		if !diag.HasErrors() {
			resolved.TerraformVersion = val.AsString()
		}
	}
	//
	autoPlanValue, ok := locals["atlantis_autoplan"]
	if ok {
		val, diag := autoPlanValue.Expr.Value(nil)
		if !diag.HasErrors() {
			hasValue := val.True()
			resolved.AutoPlan = &hasValue
		}

	}

	skipValue, ok := locals["atlantis_skip"]
	if ok {
		val, diag := skipValue.Expr.Value(nil)
		if !diag.HasErrors() {
			hasValue := val.True()
			resolved.Skip = &hasValue
		}
	}

	applyReqs, ok := locals["atlantis_apply_requirements"]
	if ok {
		val, diag := applyReqs.Expr.Value(nil)
		if !diag.HasErrors() {
			resolved.ApplyRequirements = []string{}
			it := val.ElementIterator()
			for it.Next() {
				_, val := it.Element()
				resolved.ApplyRequirements = append(resolved.ApplyRequirements, val.AsString())
			}
		}

	}

	extraDependencies, ok := locals["extra_atlantis_dependencies"]
	if ok {
		val, diag := extraDependencies.Expr.Value(nil)
		if !diag.HasErrors() {
			it := val.ElementIterator()
			for it.Next() {
				_, val := it.Element()
				resolved.ExtraAtlantisDependencies = append(resolved.ExtraAtlantisDependencies, filepath.ToSlash(val.AsString()))
			}
		}
	}

	return resolved
}
