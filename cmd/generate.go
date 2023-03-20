package cmd

import (
	"github.com/hashicorp/terraform/configs"
	"regexp"
	"sort"

	log "github.com/sirupsen/logrus"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"

	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Terragrunt imports can be relative or absolute
// This makes relative paths absolute
func makePathAbsolute(path string, parentPath string) string {
	if strings.HasPrefix(path, filepath.ToSlash(gitRoot)) {
		return path
	}

	return filepath.Join(parentPath, path)
}

var requestGroup singleflight.Group

func uniqueStrings(str []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range str {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// sliceUnion takes two slices of strings and produces a union of them, containing only unique values
func sliceUnion(a, b []string) []string {
	m := make(map[string]bool)

	for _, item := range a {
		m[item] = true
	}

	for _, item := range b {
		if _, ok := m[item]; !ok {
			a = append(a, item)
		}
	}
	return a
}

// Parses the terragrunt config at `path` to find all modules it depends on
func getDependencies(module *configs.Module, locals ResolvedLocals) ([]string, error) {
	res, err, _ := requestGroup.Do(module.SourceDir, func() (interface{}, error) {

		dependencies := []string{}
		// Get deps from locals
		if locals.ExtraAtlantisDependencies != nil {
			dependencies = sliceUnion(dependencies, locals.ExtraAtlantisDependencies)
		}

		// Get deps from locally used modules
		if !ignoreLocalSubModules {
			ls, err := parseTerraformLocalModuleSource(module)
			if err != nil {
				return nil, err
			}
			sort.Strings(ls)

			dependencies = append(dependencies, ls...)
		}

		// Filter out and dependencies that are the empty string
		nonEmptyDeps := []string{}
		for _, dep := range dependencies {
			if dep != "" {
				childDepAbsPath := dep
				if !filepath.IsAbs(childDepAbsPath) {
					childDepAbsPath = makePathAbsolute(dep, module.SourceDir)
				}
				childDepAbsPath = filepath.ToSlash(childDepAbsPath)
				nonEmptyDeps = append(nonEmptyDeps, childDepAbsPath)
			}
		}
		return dependencies, nil
	})

	if res != nil {
		return res.([]string), err
	} else {
		return nil, err
	}
}

// Creates an AtlantisProject for a directory
func createProject(path string) (*AtlantisProject, error) {
	// Errors here are only warnings that we can live with. All these modules have already been loaded in dir walk phase
	rootModule, _ := configs.NewParser(nil).LoadConfigDir(path)

	absoluteSourceDir := rootModule.SourceDir + string(filepath.Separator)

	locals := resolveLocals(rootModule)

	// If `atlantis_skip` is true on the module, then do not produce a project for it
	if locals.Skip != nil && *locals.Skip {
		return nil, nil
	}

	dependencies := []string{}
	dependencies, err := getDependencies(rootModule, locals)
	if err != nil {
		return nil, err
	}

	// dependencies being nil is a sign from `getDependencies` that this project should be skipped
	if dependencies == nil {
		return nil, nil
	}

	// All dependencies depend on their own .hcl file, and any tf files in their directory
	relativeDependencies := autoPlanFileList

	// Add other dependencies based on their relative paths. We always want to output with Unix path separators
	for _, dependencyPath := range dependencies {
		absolutePath := dependencyPath
		if !filepath.IsAbs(absolutePath) {
			absolutePath = makePathAbsolute(dependencyPath, rootModule.SourceDir)
		}
		relativePath, err := filepath.Rel(absoluteSourceDir, absolutePath)
		if err != nil {
			return nil, err
		}

		relativeDependencies = append(relativeDependencies, filepath.ToSlash(relativePath))
	}

	// Clean up the relative path to the format Atlantis expects
	relativeSourceDir := strings.TrimPrefix(absoluteSourceDir, gitRoot)
	relativeSourceDir = strings.TrimSuffix(relativeSourceDir, string(filepath.Separator))
	if relativeSourceDir == "" {
		relativeSourceDir = "."
	}

	workflow := defaultWorkflow
	if locals.AtlantisWorkflow != "" {
		workflow = locals.AtlantisWorkflow
	}

	applyRequirements := &defaultApplyRequirements
	if len(defaultApplyRequirements) == 0 {
		applyRequirements = nil
	}
	if locals.ApplyRequirements != nil {
		applyRequirements = &locals.ApplyRequirements
	}

	resolvedAutoPlan := autoPlan
	if locals.AutoPlan != nil {
		resolvedAutoPlan = *locals.AutoPlan
	}

	terraformVersion := defaultTerraformVersion
	if locals.TerraformVersion != "" {
		terraformVersion = locals.TerraformVersion
	}

	project := &AtlantisProject{
		Dir:               filepath.ToSlash(relativeSourceDir),
		Workflow:          workflow,
		TerraformVersion:  terraformVersion,
		ApplyRequirements: applyRequirements,
		Autoplan: AutoplanConfig{
			Enabled:      resolvedAutoPlan,
			WhenModified: uniqueStrings(relativeDependencies),
		},
	}

	if locals.ExecutionOrderGroup > 0 {
		project.ExecutionOrderGroup = locals.ExecutionOrderGroup
	}

	// Terraform Cloud limits the workspace names to be less than 90 characters
	// with letters, numbers, -, and _
	// https://www.terraform.io/docs/cloud/workspaces/naming.html
	// It is not clear from documentation whether the normal workspaces have those limitations
	// However a workspace 97 chars long has been working perfectly.
	// We are going to use the same name for both workspace & project name as it is unique.
	regex := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	projectName := regex.ReplaceAllString(project.Dir, "_")

	if createProjectName {
		project.Name = projectName
	}

	if createWorkspace {
		project.Workspace = projectName
	}

	return project, nil
}

func FindRootModulesInPath(rootPath string) ([]string, error) {
	var rootModules []string

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		// Skip .terraform and .git dirs
		if info.IsDir() && (info.Name() == ".terraform" || info.Name() == ".git") {
			return filepath.SkipDir
		}

		if info.IsDir() {

			module, diag := configs.NewParser(nil).LoadConfigDir(path)
			if diag.HasErrors() && module.Backend == nil {
				log.Debugf("Failed to load module at: %s", path)
				return nil
			}

			if module.Backend == nil {
				return nil
			}
			rootModules = append(rootModules, path)
		}
		return nil
	})

	return rootModules, err
}

// Finds the absolute paths of all terragrunt.hcl files
func getAllTerraformRootModules(path string) ([]string, error) {
	// If filterPath is provided, override workingPath instead of gitRoot
	// We do this here because we want to keep the relative path structure of Terragrunt files
	// to root and just ignore the ConfigFiles
	workingPaths := []string{path}

	// filters are not working (yet) if using project hcl files (which are kind of filters by themselves)
	var err error
	if filterPath != "" {
		// get all matching folders
		workingPaths, err = filepath.Glob(filterPath)
		if err != nil {
			return nil, err
		}
	}

	uniqueConfigFilePaths := make(map[string]bool)
	orderedConfigFilePaths := []string{}
	for _, workingPath := range workingPaths {
		paths, err := FindRootModulesInPath(workingPath)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			// if path not yet seen, insert once
			if !uniqueConfigFilePaths[p] {
				orderedConfigFilePaths = append(orderedConfigFilePaths, p)
				uniqueConfigFilePaths[p] = true
			}
		}
	}

	uniqueConfigFileAbsPaths := []string{}
	for _, uniquePath := range orderedConfigFilePaths {
		uniqueAbsPath, err := filepath.Abs(uniquePath)
		if err != nil {
			return nil, err
		}
		uniqueConfigFileAbsPaths = append(uniqueConfigFileAbsPaths, uniqueAbsPath)
	}

	return uniqueConfigFileAbsPaths, nil
}

func main(cmd *cobra.Command, args []string) error {
	// Ensure the gitRoot has a trailing slash and is an absolute path
	absoluteGitRoot, err := filepath.Abs(gitRoot)
	if err != nil {
		return err
	}
	gitRoot = absoluteGitRoot + string(filepath.Separator)
	workingDirs := []string{gitRoot}

	// Read in the old config, if it already exists
	oldConfig, err := readOldConfig()
	if err != nil {
		return err
	}
	config := AtlantisConfig{
		Version:       3,
		AutoMerge:     autoMerge,
		ParallelPlan:  parallel,
		ParallelApply: parallel,
	}
	if oldConfig != nil && preserveWorkflows {
		config.Workflows = oldConfig.Workflows
	}
	if oldConfig != nil && preserveProjects {
		config.Projects = oldConfig.Projects
	}

	lock := sync.Mutex{}
	ctx := context.Background()
	errGroup, _ := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(numExecutors)

	for _, workingDir := range workingDirs {
		terraformRootModules, err := getAllTerraformRootModules(workingDir)
		if err != nil {
			return err
		}

		// Concurrently looking all dependencies
		for _, rootModule := range terraformRootModules {
			modulePath := rootModule // https://golang.org/doc/faq#closures_and_goroutines

			err := sem.Acquire(ctx, 1)
			if err != nil {
				return err
			}

			errGroup.Go(func() error {
				defer sem.Release(1)
				project, err := createProject(modulePath)
				if err != nil {
					return err
				}
				// if project and err are nil then skip this project
				if err == nil && project == nil {
					return nil
				}

				// Lock the list as only one goroutine should be writing to config.Projects at a time
				lock.Lock()
				defer lock.Unlock()

				// When preserving existing projects, we should update existing blocks instead of creating a
				// duplicate, when generating something which already has representation
				if preserveProjects {
					updateProject := false

					// TODO: with Go 1.19, we can replace for loop with slices.IndexFunc for increased performance
					for i := range config.Projects {
						if config.Projects[i].Dir == project.Dir {
							updateProject = true
							log.Info("Updated project for ", modulePath)
							config.Projects[i] = *project

							// projects should be unique, let's exit for loop for performance
							// once first occurrence is found and replaced
							break
						}
					}

					if !updateProject {
						log.Info("Created project for ", modulePath)
						config.Projects = append(config.Projects, *project)
					}
				} else {
					log.Info("Created project for ", modulePath)
					config.Projects = append(config.Projects, *project)
				}

				return nil
			})
		}

		if err := errGroup.Wait(); err != nil {
			return err
		}
	}

	// Sort the projects in config by Dir
	sort.Slice(config.Projects, func(i, j int) bool { return config.Projects[i].Dir < config.Projects[j].Dir })

	if executionOrderGroups {
		projectsMap := make(map[string]*AtlantisProject, len(config.Projects))
		for i := range config.Projects {
			projectsMap[config.Projects[i].Dir] = &config.Projects[i]
		}

		// Compute order groups in the cycle to avoid incorrect values in cascade dependencies
		hasChanges := true
		for i := 0; hasChanges && i <= len(config.Projects); i++ {
			hasChanges = false
			for _, project := range config.Projects {
				executionOrderGroup := 0
				// choose order group based on dependencies
				for _, dep := range project.Autoplan.WhenModified {
					depPath := filepath.Dir(filepath.Join(project.Dir, dep))
					if depPath == project.Dir {
						// skip dependency on oneself
						continue
					}

					depProject, ok := projectsMap[depPath]
					if !ok {
						// skip not project dependencies
						continue
					}
					if depProject.ExecutionOrderGroup+1 > executionOrderGroup {
						executionOrderGroup = depProject.ExecutionOrderGroup + 1
					}
				}
				if projectsMap[project.Dir].ExecutionOrderGroup != executionOrderGroup {
					projectsMap[project.Dir].ExecutionOrderGroup = executionOrderGroup
					// repeat the main cycle when changed some project
					hasChanges = true
				}
			}
		}

		if hasChanges {
			// Should be unreachable
			log.Warn("Computing execution_order_groups failed. Probably cycle exists")
		}

		// Sort by execution_order_group
		sort.Slice(config.Projects, func(i, j int) bool {
			if config.Projects[i].ExecutionOrderGroup == config.Projects[j].ExecutionOrderGroup {
				return config.Projects[i].Dir < config.Projects[j].Dir
			}
			return config.Projects[i].ExecutionOrderGroup < config.Projects[j].ExecutionOrderGroup
		})
	}

	// Convert config to YAML string
	yamlBytes, err := yaml.Marshal(&config)
	if err != nil {
		return err
	}

	// Ensure newline characters are correct on windows machines, as the json encoding function in the stdlib
	// uses "\n" for all newlines regardless of OS: https://github.com/golang/go/blob/master/src/encoding/json/stream.go#L211-L217
	yamlString := string(yamlBytes)
	if strings.Contains(runtime.GOOS, "windows") {
		yamlString = strings.ReplaceAll(yamlString, "\n", "\r\n")
	}

	// Write output
	if len(outputPath) != 0 {
		ioutil.WriteFile(outputPath, []byte(yamlString), 0644)
	} else {
		log.Println(yamlString)
	}

	return nil
}

var gitRoot string
var autoPlan bool
var autoPlanFileList []string
var autoMerge bool
var ignoreLocalSubModules bool
var localSubModulesExclude []string
var parallel bool
var createWorkspace bool
var createProjectName bool
var defaultTerraformVersion string
var defaultWorkflow string
var filterPath string
var outputPath string
var preserveWorkflows bool
var preserveProjects bool
var defaultApplyRequirements []string
var numExecutors int64
var executionOrderGroups bool

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Makes atlantis config",
	Long:  `Logs Yaml representing Atlantis config to stderr`,
	RunE:  main,
}

func init() {
	rootCmd.AddCommand(generateCmd)

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	generateCmd.PersistentFlags().BoolVar(&autoPlan, "autoplan", false, "Enable auto plan. Default is disabled")
	generateCmd.PersistentFlags().BoolVar(&autoMerge, "automerge", false, "Enable auto merge. Default is disabled")
	generateCmd.PersistentFlags().BoolVar(&parallel, "parallel", true, "Enables plans and applys to happen in parallel. Default is enabled")
	generateCmd.PersistentFlags().BoolVar(&ignoreLocalSubModules, "ignore-local-sub-modules", false, "When true, dependencies found in `dependency` blocks will be ignored")
	generateCmd.PersistentFlags().StringSliceVar(&localSubModulesExclude, "local-sub-modules-exclude", []string{}, "Local sub modules that should be excluded from being added to 'when_modified' if --ignore-local-sub-modules is false (default)")
	generateCmd.PersistentFlags().StringSliceVar(&autoPlanFileList, "autoplan-file-list", []string{"*.tf*"}, "Glob of module-local files that should be included in auto plan")
	generateCmd.PersistentFlags().BoolVar(&createWorkspace, "create-workspace", false, "Use different workspace for each project. Default is use default workspace")
	generateCmd.PersistentFlags().BoolVar(&preserveWorkflows, "preserve-workflows", true, "Preserves workflows from old output files. Default is true")
	generateCmd.PersistentFlags().BoolVar(&preserveProjects, "preserve-projects", false, "Preserves projects from old output files to enable incremental builds. Default is false")
	generateCmd.PersistentFlags().StringVar(&defaultWorkflow, "workflow", "", "Name of the workflow to be customized in the atlantis server. Default is to not set")
	generateCmd.PersistentFlags().StringSliceVar(&defaultApplyRequirements, "apply-requirements", []string{}, "Requirements that must be satisfied before `atlantis apply` can be run. Currently the only supported requirements are `approved` and `mergeable`. Can be overridden by locals")
	generateCmd.PersistentFlags().StringVar(&outputPath, "output", "", "Path of the file where configuration will be generated. Default is not to write to file")
	generateCmd.PersistentFlags().StringVar(&filterPath, "filter", "", "Path or glob expression to the directory you want scope down the config for. Default is all files in root")
	generateCmd.PersistentFlags().StringVar(&gitRoot, "root", pwd, "Path to the root directory of the git repo you want to build config for. Default is current dir")
	generateCmd.PersistentFlags().StringVar(&defaultTerraformVersion, "terraform-version", "", "Default terraform version to specify for all modules. Can be overriden by locals")
	generateCmd.PersistentFlags().Int64Var(&numExecutors, "num-executors", 15, "Number of executors used for parallel generation of projects. Default is 15")
	generateCmd.PersistentFlags().BoolVar(&executionOrderGroups, "execution-order-groups", false, "Computes execution_order_groups for projects")
}

// Runs a set of arguments, returning the output
func RunWithFlags(filename string, args []string) ([]byte, error) {
	rootCmd.SetArgs(args)
	rootCmd.Execute()

	return ioutil.ReadFile(filename)
}
