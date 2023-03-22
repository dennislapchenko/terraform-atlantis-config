<p align="center">
  <img alt="Terragrunt Atlantis Config by Transcend" src="https://user-images.githubusercontent.com/7354176/78756035-f9863480-792e-11ea-96d3-d4ffe50e0269.png"/>
</p>
<h1 align="center">Terraform Atlantis Config</h1>
<p align="center">
  <strong>Generate Atlantis Config for Terraform projects.</strong>
</p>
<br />

## A fork of [terragrunt-atlantis-config](https://github.com/transcend-io/terragrunt-atlantis-config) made to work with vanilla Terraform
Original repo was forked in order to quickly solve the need to generate atlantis configs for vanilla Terraform the same way its done for Terragrunt.
No existing tool could do so in the same manner, especially giving the ability to alter config via `locals`.
The core logic was rewritten, using the initial work done by transcend-io.

#### TODO:
- [ ] Adapt tests to Terraform
- [ ] Fully adapt this README.md to remove terragrunt mentions
- [ ] Find a way how to use latest Terraform versions. Currently impossible because `hashicorp/terraform/configs` was moved to internal, however same (locals) functionality was not replicated in `terraform-config-inspect`


# Up To Date Doc:


## All Locals

Another way to customize the output is to use object `atlantis` in `locals` block in your terraform modules. These settings will only affect the current module/

| Locals Name                    | Description                                                                                                                                                    | type         |
|--------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------|
| `atlantis.workflow`            | The custom atlantis workflow name to use for a module                                                                                                          | string       |
| `atlantis.apply_requirements`  | The custom `apply_requirements` array to use for a module                                                                                                      | list(string) |
| `atlantis.terraform_version`   | Allows overriding the `--terraform-version` flag for a single module                                                                                           | string       |
| `atlantis.autoplan`            | Allows overriding the `--autoplan` flag for a single module                                                                                                    | bool         |
| `atlantis.skip`                | If true on a child module, that module will not appear in the output.<br>If true on a parent module, none of that parent's children will appear in the output. | bool         |
| `atlantis.extra__dependencies` | See [Extra dependencies](https://github.com/transcend-io/terragrunt-atlantis-config#extra-dependencies)                                                        | list(string) |
| `atlantis.execution_order_group`  | See [Execution order group](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html#order-of-planning-applying)                                                         | number        |
Full example:
```hcl
locals {
  atlantis = {
    workflow = "my-workflow"
    apply_requirements = ["approved"]
    terraform_version = "1.3.7"
    autoplan = true
    skip = false
    extra_dependencies = ["../../common-config.yaml"]
    execution_order_group = 1
  }
}
```

# Out of Date Doc
## What is this?
All below README contents are yet to be fully refactored, but most of it applied to this tool too.
[Atlantis](https://runatlantis.io) is an awesome tool for Terraform pull request automation. Each repo can have a YAML config file that defines Terraform module dependencies, so that PRs that affect dependent modules will automatically generate `terraform plan`s for those modules.

This tool creates Atlantis YAML configurations for Terraform projects by:

- Finding all root modules that contain a `backend{}` configuration in a repo
- Evaluating their `module` and `locals` blocks to find their dependencies
- Constructing and logging YAML in Atlantis' config spec that reflects the graph

This is especially useful for organizations that use monorepos for their Terraform config, and have thousands of lines of config.

## Integrate into your Atlantis Server

The recommended way to use this tool is to install it onto your Atlantis server, and then use a [Pre-Workflow hook](https://www.runatlantis.io/docs/pre-workflow-hooks.html#pre-workflow-hooks) to run it after every clone. This way, Atlantis can automatically determine what modules should be planned/applied for any change to your repository.

To get started, add a `pre_workflow_hooks` field to your `repos` section of your [server-side repo config](https://www.runatlantis.io/docs/server-side-repo-config.html#do-i-need-a-server-side-repo-config-file):

```json
{
  "repos": [
    {
      "id": "<your_github_repo>",
      "workflow": "default",
      "pre_workflow_hooks": [
        {
          "run": "terragrunt-atlantis-config generate --output atlantis.yaml --autoplan --parallel --create-workspace"
        }
      ]
    }
  ]
}
```

Then, make sure `terragrunt-atlantis-config` is present on your Atlantis server. There are many different ways to configure a server, but this example in [Packer](https://www.packer.io/) should show the bash commands you'll need just about anywhere:

```hcl
variable "terragrunt_atlantis_config_version" {
  default = "1.16.0"
}

build {
  // ...
  provisioner "shell" {
    inline = [
      "wget https://github.com/transcend-io/terragrunt-atlantis-config/releases/download/v${var.terragrunt_atlantis_config_version}/terragrunt-atlantis-config_${var.terragrunt_atlantis_config_version}_linux_amd64.tar.gz",
      "sudo tar xf terragrunt-atlantis-config_${var.terragrunt_atlantis_config_version}_linux_amd64.tar.gz",
      "sudo mv terragrunt-atlantis-config_${var.terragrunt_atlantis_config_version}_linux_amd64/terragrunt-atlantis-config_${var.terragrunt_atlantis_config_version}_linux_amd64 terragrunt-atlantis-config",
      "sudo install terragrunt-atlantis-config /usr/local/bin",
    ]
    inline_shebang = "/bin/bash -e"
  }
  // ...
}
```

and just like that, your developers should never have to worry about an `atlantis.yaml` file, or even need to know what it is.

## Extra dependencies

For basic cases, this tool can sniff out all dependencies in a module. However, you may have times when you want to add in additional dependencies such as:

- You use Terragrunt's `read_terragrunt_config` function in your locals, and want to depend on the read file
- Your Terragrunt module should be run anytime some non-terragrunt file is updated, such as a Dockerfile or Packer template
- You want to run _all_ modules any time your product has a major version bump
- You believe a module should be reapplied any time some other file or directory is updated

In these cases, you can customize the `locals` block in that Terragrunt module to have a field named `extra_atlantis_dependencies` with a list
of values you want included in the config, such as:

```hcl
locals {
  extra_atlantis_dependencies = [
    "some_extra_dep",
    find_in_parent_folders(".gitignore")
  ]
}
```

In your `atlantis.yaml` file, you will end up seeing output like:

```yaml
- autoplan:
    enabled: false
    when_modified:
      - "*.hcl"
      - "*.tf*"
      - some_extra_dep
      - ../../.gitignore
  dir: example-setup/extra_dependency
```

If you specify `extra_atlantis_dependencies` in the parent Terragrunt module, they will be merged with the child dependencies using the following rules:

1. Any function in a parent will be evaluated from the child's directory. So you can use `get_parent_terragrunt_dir()` and other functions like you normally would in terragrunt.
2. Absolute paths will work as they would in a child module, and the path in the output will be relative from the child module to the absolute path
3. Relative paths, like the string `"foo.json"`, will be evaluated as relative to the Child module. This means that if you need something relative to the parent module, you should use something like `"${get_parent_terragrunt_dir()}/foo.json"`

## All Flags

One way to customize the behavior of this module is through CLI flag values passed in at runtime. These settings will apply to all modules.

| Flag Name                    | Description                                                                                                                                                                     | Default Value     |
|------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------|
| `--autoplan`                 | The default value for autoplan settings. Can be overriden by locals.                                                                                                            | false             |
| `--automerge`                | Enables the automerge setting for a repo.                                                                                                                                       | false             |
| `--parallel`                 | Enables `plan`s and `apply`s to happen in parallel. Will typically be used with `--create-workspace`                                                                            | true              |
| `--create-workspace`         | Use different auto-generated workspace for each project. Default is use default workspace for everything                                                                        | false             |
| `--create-project-name`      | Add different auto-generated name for each project                                                                                                                              | false             |
| `--preserve-workflows`       | Preserves workflows from old output files. Useful if you want to define your workflow definitions on the client side                                                            | true              |
| `--preserve-projects`        | Preserves projects from old output files. Useful for incremental builds using `--filter`                                                                                        | false             |
| `--workflow`                 | Name of the workflow to be customized in the atlantis server. If empty, will be left out of output                                                                              | ""                |
| `--apply-requirements`       | Requirements that must be satisfied before `atlantis apply` can be run. Currently the only supported requirements are `approved` and `mergeable`. Can be overridden by locals   | []                |
| `--output`                   | Path of the file where configuration will be generated. Typically, you want a file named "atlantis.yaml". Default is to write to `stdout`.                                      | ""                |
| `--root`                     | Path to the root directory of the git repo you want to build config for.                                                                                                        | current directory |
| `--terraform-version`        | Default terraform version to specify for all modules. Can be overriden by locals                                                                                                | ""                |
| `--num-executors`            | Number of executors used for parallel generation of projects. Default is 15                                                                                                     | 15                |
| `--execution-order-groups`   | Computes execution_order_group for projects                                                                                                                                     | false             |



## Separate workspace for parallel plan and apply

Atlantis added support for running plan and apply parallel in [v0.13.0](https://github.com/runatlantis/atlantis/releases/tag/v0.13.0).

To use this feature, projects have to be separated in different workspaces, and the `create-workspace` flag enables this by concatenating the project path as the
name of the workspace.

As an example, project `${git_root}/stage/app/terragrunt.hcl` will have the name `stage_app` as workspace name. This flag should be used along with `parallel` to enable parallel plan and apply:

```bash
terraform-atlantis-config generate --output atlantis.yaml --parallel --create-workspace
```

Enabling this feature may consume more resources like cpu, memory, network, and disk, as each workspace will now be cloned separately by atlantis.

As when defining the workspace this info is also needed when running `atlantis plan/apply -d ${git_root}/stage/app -w stage_app` to run the command on specific directory,
you can also use the `atlantis plan/apply -p stage_app` in case you have enabled the `create-project-name` cli argument (it is `false` by default).

## Rules for merging config

Each terragrunt module can have locals, but can also have zero to many `include` blocks that can specify parent terragrunt files that can also have locals.

In most cases (for string/boolean locals), the primary terragrunt module has the highest precedence, followed by the locals in the lowest appearing `include` block, etc. all the way until the lowest precedence at the locals in the first `include` block to appear.

However, there is one exception where the values are merged, which is the `atlantis_extra_dependencies` local. For this local, all values are appended to one another. This way, you can have `include` files declare their own dependencies.

## Local Installation and Usage

You can install this tool locally to checkout what kinds of config it will generate for your repo, though in production it is recommended to [install this tool directly onto your Atlantis server](##integrate-into-your-atlantis-server)

Recommended: Install any version via go install:

```bash
go install github.com/transcend-io/terragrunt-atlantis-config@v1.16.0
```

This module officially supports golang versions v1.13, v1.14, v1.15, and v1.16, tested on CircleCI with each build
This module also officially supports both Windows and Nix-based file formats, tested on CircleCI with each build

Usage Examples (see below sections for all options):

```bash
# From the root of your repo
terragrunt-atlantis-config generate

# or from anywhere
terragrunt-atlantis-config generate --root /some/path/to/your/repo/root

# output to a file
terragrunt-atlantis-config generate --autoplan --output ./atlantis.yaml
```

Finally, check the log output (or your output file) for the YAML.

## Contributing

To test any changes you've made, run `make gotestsum` (or `make test` for standard golang testing).

Once all your changes are passing and your PR is reviewed, a merge into `master` will trigger a Github Actions job to build the new binary, test it, and deploy it's artifacts to Github Releases along with checksums.

You can then open a PR on our homebrew tap similar to https://github.com/transcend-io/homebrew-tap/pull/4, and as soon as that merges your code will be released. Homebrew is not updated for every release, as Github is the primary artifact store.

## Contributors

<img src="./CONTRIBUTORS.svg">

## Stargazers over time

[![Stargazers over time](https://starchart.cc/dennislapchenko/terraform-atlantis-config.svg)](https://starchart.cc/dennislapchenko/terraform-atlantis-config)

## License

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Ftranscend-io%2Fterragrunt-atlantis-config.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Ftranscend-io%2Fterragrunt-atlantis-config?ref=badge_large)
